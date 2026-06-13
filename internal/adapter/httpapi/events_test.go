package httpapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

// fakeSSEServer writes one SSE event then closes the connection.
func fakeSSEServer(t *testing.T, events []string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var connects atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connects.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for _, ev := range events {
			_, _ = fmt.Fprint(w, ev)
			flusher.Flush()
		}
		// Close connection by returning (stream ends)
	}))
	t.Cleanup(srv.Close)
	return srv, &connects
}

func TestEventsChangedEventInvalidatesAndNotifies(t *testing.T) {
	srv, _ := fakeSSEServer(t, []string{
		"event: changed\ndata: {\"resource\":\"worktime\"}\n\n",
	})

	var invalidated []string
	invalidate := func(resource string) {
		invalidated = append(invalidated, resource)
	}

	// Build a minimal Client pointing at the fake server
	tokens := &memTokens{ok: true, tok: ports.Tokens{AccessToken: "test-token"}}
	cli := httpapi.New(httpapi.Config{
		BaseURL: srv.URL,
		Tokens:  tokens,
		Slot:    "test",
		Version: "dev",
	})

	ev := httpapi.NewEvents(cli, invalidate)
	ev.Start(t.Context())
	defer ev.Stop()

	// Wait for Changed signal (with timeout)
	select {
	case <-ev.Changed():
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Changed signal")
	}

	// "worktime" must be in invalidated (from the explicit event)
	found := false
	for _, r := range invalidated {
		if r == "worktime" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'worktime' in invalidated resources, got %v", invalidated)
	}
}

func TestEventsReconnect(t *testing.T) {
	// Server closes immediately; client should reconnect (attempt >= 2).
	var connects atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connects.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		// Return immediately — closes stream, triggering reconnect
	}))
	t.Cleanup(srv.Close)

	tokens := &memTokens{ok: true, tok: ports.Tokens{AccessToken: "test-token"}}
	cli := httpapi.New(httpapi.Config{
		BaseURL: srv.URL,
		Tokens:  tokens,
		Slot:    "test",
		Version: "dev",
	})

	ev := httpapi.NewEvents(cli, func(string) {})
	ev.Start(t.Context())
	defer ev.Stop()

	// Wait until the client has connected at least twice
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if connects.Load() >= 2 {
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("expected at least 2 connects, got %d", connects.Load())
}
