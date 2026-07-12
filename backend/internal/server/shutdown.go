package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/palhelm/palhelm/internal/palworld"
)

// CountdownStage describes when a warning is sent relative to countdown start.
type CountdownStage struct {
	After     time.Duration
	Remaining time.Duration
}

// CountdownStages returns applicable warning stages, skipping thresholds already past.
func CountdownStages(wait time.Duration) []CountdownStage {
	thresholds := []time.Duration{10 * time.Minute, 5 * time.Minute, time.Minute, 30 * time.Second, 10 * time.Second}
	out := []CountdownStage{}
	for _, remaining := range thresholds {
		if remaining <= wait {
			out = append(out, CountdownStage{After: wait - remaining, Remaining: remaining})
		}
	}
	return out
}

type shutdowner interface {
	Announce(context.Context, string) error
	Shutdown(context.Context, int, string) error
}
type orchestrator struct {
	client shutdowner
	mu     sync.Mutex
	state  string
	cancel context.CancelFunc
	now    func() time.Time
	after  func(time.Duration) <-chan time.Time
}

func newOrchestrator(c *palworld.Client) *orchestrator {
	return &orchestrator{client: c, state: "running", now: time.Now, after: time.After}
}
func (o *orchestrator) State() string { o.mu.Lock(); defer o.mu.Unlock(); return o.state }
func (o *orchestrator) Start(parent context.Context, wait time.Duration, message string, countdown bool) error {
	o.mu.Lock()
	if o.state == "countdown" || o.state == "stopping" {
		o.mu.Unlock()
		return fmt.Errorf("shutdown already pending")
	}
	ctx, cancel := context.WithCancel(parent)
	o.cancel = cancel
	if countdown && wait > 0 {
		o.state = "countdown"
	} else {
		o.state = "stopping"
	}
	o.mu.Unlock()
	go o.run(ctx, wait, message, countdown)
	return nil
}
func (o *orchestrator) run(ctx context.Context, wait time.Duration, message string, countdown bool) {
	if !countdown {
		o.set("stopping")
		if err := o.client.Shutdown(ctx, int(wait.Seconds()), message); err != nil {
			o.set("running")
		}
		return
	}
	started := o.now()
	for _, stage := range CountdownStages(wait) {
		timer := o.after(maxDuration(0, started.Add(stage.After).Sub(o.now())))
		select {
		case <-ctx.Done():
			o.set("running")
			return
		case <-timer:
		}
		text := message
		if text == "" {
			text = "Server shutdown"
		}
		_ = o.client.Announce(ctx, fmt.Sprintf("%s in %s", text, formatRemaining(stage.Remaining)))
	}
	timer := o.after(maxDuration(0, started.Add(wait).Sub(o.now())))
	select {
	case <-ctx.Done():
		o.set("running")
		return
	case <-timer:
	}
	o.set("stopping")
	if err := o.client.Shutdown(ctx, 10, message); err != nil {
		o.set("running")
	}
}
func (o *orchestrator) Cancel() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state != "countdown" || o.cancel == nil {
		return false
	}
	o.cancel()
	o.cancel = nil
	return true
}
func (o *orchestrator) set(v string) { o.mu.Lock(); o.state = v; o.mu.Unlock() }
func formatRemaining(v time.Duration) string {
	if v >= time.Minute {
		return fmt.Sprintf("%dm", int(v.Minutes()))
	}
	return fmt.Sprintf("%ds", int(v.Seconds()))
}
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
