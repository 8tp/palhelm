package server

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakeShutdownClient struct {
	mu        sync.Mutex
	announces []string
	done      chan struct{}
}

func (f *fakeShutdownClient) Announce(_ context.Context, v string) error {
	f.mu.Lock()
	f.announces = append(f.announces, v)
	f.mu.Unlock()
	return nil
}
func (f *fakeShutdownClient) Shutdown(context.Context, int, string) error { close(f.done); return nil }

func TestCountdownStageMath(t *testing.T) {
	got := CountdownStages(6 * time.Minute)
	want := []CountdownStage{{After: time.Minute, Remaining: 5 * time.Minute}, {After: 5 * time.Minute, Remaining: time.Minute}, {After: 330 * time.Second, Remaining: 30 * time.Second}, {After: 350 * time.Second, Remaining: 10 * time.Second}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestCountdownWithFakeClock(t *testing.T) {
	client := &fakeShutdownClient{done: make(chan struct{})}
	now := time.Unix(100, 0)
	var waits []time.Duration
	o := &orchestrator{client: client, state: "running", now: func() time.Time { return now }, after: func(d time.Duration) <-chan time.Time {
		waits = append(waits, d)
		ch := make(chan time.Time, 1)
		now = now.Add(d)
		ch <- now
		return ch
	}}
	if err := o.Start(context.Background(), 70*time.Second, "Restart", true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatal("countdown did not finish")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.announces) != 3 {
		t.Fatalf("announces=%v", client.announces)
	}
	if !reflect.DeepEqual(waits, []time.Duration{10 * time.Second, 30 * time.Second, 20 * time.Second, 10 * time.Second}) {
		t.Fatalf("waits=%v", waits)
	}
}
