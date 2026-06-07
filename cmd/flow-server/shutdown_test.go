package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// TestIntegration_GracefulShutdown_DrainsActiveRequests boots a real
// *http.Server on a random port, fires an in-flight slow request, then
// cancels the run context (the signal-equivalent). The test asserts the
// slow request completes (drain worked) and runServer returns nil within
// the 1s drain budget.
//
// We test runServer rather than run() because run() depends on a real
// OIDC discovery endpoint and SQLite path — that scope belongs in a
// dedicated e2e suite, not a unit test. runServer is the actual seam
// that owns the lifecycle (ListenAndServe goroutine, shutdown timeout,
// beforeShutdown hooks), so covering it gives us the regression
// guarantee we need.
func TestIntegration_GracefulShutdown_DrainsActiveRequests(t *testing.T) {
	t.Parallel()

	// Bind :0 ourselves so the test learns the actual port — we use it
	// for both the in-flight request and the post-drain probe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	// Signal channel: handler closes it once it has started writing the
	// response, so the test knows the request is in-flight before we
	// trigger shutdown.
	requestStarted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(requestStarted)
		// Sleep that's well under the drain budget — gives the test a
		// window to send SIGTERM-equivalent while the request is alive.
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "done")
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 1 * time.Second,
	}

	// runServer expects a fresh listener via srv.ListenAndServe — we
	// already opened one for the address, so close it and let runServer
	// re-open via the srv.Addr field. This matches how main wires it.
	if err := ln.Close(); err != nil {
		t.Fatalf("ln close: %v", err)
	}
	srv.Addr = addr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// beforeShutdown hook to verify the SSE-ticker-equivalent fires.
	var hookFired atomic.Bool
	hook := func() { hookFired.Store(true) }

	runDone := make(chan error, 1)
	go func() {
		runDone <- runServer(ctx, srv, 1*time.Second, logger, hook)
	}()

	// Wait for the server to be accepting connections. We poll instead
	// of sleeping because :0-bound listeners are ready immediately, but
	// the goroutine boundary plus ListenAndServe setup can lag a few ms.
	if err := waitForServerReady(addr, 1*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	// Fire the slow request in another goroutine.
	slowResp := make(chan *http.Response, 1)
	slowErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			slowErr <- err
			return
		}
		slowResp <- resp
	}()

	// Wait until the handler has actually started — we want a true
	// in-flight request, not a queued connection.
	select {
	case <-requestStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("slow handler never started")
	}

	// Trigger shutdown — equivalent to SIGTERM arriving.
	cancel()

	// Slow request must complete cleanly because Shutdown drains
	// in-flight requests instead of severing them.
	select {
	case resp := <-slowResp:
		if resp.StatusCode != http.StatusOK {
			t.Errorf("slow request: status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Errorf("slow request body read: %v", err)
		}
		if string(body) != "done" {
			t.Errorf("slow request body = %q, want %q", string(body), "done")
		}
	case err := <-slowErr:
		t.Errorf("slow request errored during drain: %v", err)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("slow request did not complete within drain budget")
	}

	// runServer must return nil (clean drain) within the budget.
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("runServer returned error: %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("runServer did not return within drain budget")
	}

	if !hookFired.Load() {
		t.Error("beforeShutdown hook was not invoked (SSE ticker cleanup would leak)")
	}

	// Post-shutdown probe: the server should no longer accept new
	// connections. A successful Dial means the listener was never
	// closed, which would be a regression.
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Error("server still accepting connections after shutdown")
	}
}

// TestIntegration_GracefulShutdown_DrainTimeout verifies that a request
// outlasting the drain budget causes runServer to return a non-nil
// error — main maps that to a non-zero exit so K8s sees the failure.
func TestIntegration_GracefulShutdown_DrainTimeout(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	requestStarted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/stuck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(requestStarted)
		// Intentionally outlast the 100ms drain budget — but bail out
		// when the test ends so we don't leak goroutines.
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 1 * time.Second,
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("ln close: %v", err)
	}
	srv.Addr = addr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	runDone := make(chan error, 1)
	go func() {
		runDone <- runServer(ctx, srv, 100*time.Millisecond, logger)
	}()

	if err := waitForServerReady(addr, 1*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	go func() { _, _ = http.Get("http://" + addr + "/stuck") }()

	select {
	case <-requestStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("stuck handler never started")
	}

	cancel()

	select {
	case err := <-runDone:
		if err == nil {
			t.Error("runServer returned nil; expected drain-timeout error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("runServer error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("runServer did not return within budget")
	}
}

// waitForServerReady polls addr until a TCP dial succeeds or the timeout
// expires. Polling instead of sleeping keeps the test fast on a healthy
// runner and resilient on a loaded one.
func waitForServerReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("server not ready within timeout")
}
