package server

import (
	"encoding/json"
	"sync"
)

type message struct {
	event string
	data  []byte
}

// Hub fans live messages out to SSE subscribers without blocking pollers.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan message]struct{}
	done    chan struct{}
	close   sync.Once
}

// NewHub creates an empty fan-out hub.
func NewHub() *Hub { return &Hub{clients: make(map[chan message]struct{}), done: make(chan struct{})} }

// Publish serializes and broadcasts an SSE event.
func (h *Hub) Publish(event string, value any) {
	select {
	case <-h.done:
		return
	default:
	}
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- message{event, b}:
		default:
		}
	}
}

// Close asks every SSE subscriber to leave without waiting for the HTTP
// server's shutdown deadline. It is safe to call more than once.
func (h *Hub) Close() { h.close.Do(func() { close(h.done) }) }

func (h *Hub) closed() <-chan struct{} { return h.done }
func (h *Hub) subscribe() (chan message, func()) {
	ch := make(chan message, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() { h.mu.Lock(); delete(h.clients, ch); h.mu.Unlock() }
}
