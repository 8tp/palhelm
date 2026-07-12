package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloseStreamsEndsSSE(t *testing.T) {
	s := &Server{hub: NewHub()}
	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil).WithContext(context.Background())
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.eventStream(rr, req)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		s.hub.mu.RLock()
		clients := len(s.hub.clients)
		s.hub.mu.RUnlock()
		if clients == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SSE handler did not subscribe")
		}
		time.Sleep(time.Millisecond)
	}

	s.CloseStreams()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not exit after lifecycle close")
	}
}
