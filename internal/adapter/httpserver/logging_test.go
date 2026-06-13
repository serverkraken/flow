package httpserver

// logging_test.go — Plan F · Task 7.
//
// Coverage for the structured JSON request-log middleware:
//
//   - writes one log line per request
//   - includes method, path, status, dur, remote attributes
//   - skips /metrics, /healthz, /readyz, /favicon.ico, /static/*
//   - includes user_sub when the request is authenticated
//   - uses the provided logger (not slog.Default()) when passed
//   - falls back to slog.Default() when nil

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// newCapturingLogger returns an slog.Logger that writes JSON lines into
// the supplied buffer. Uses LevelDebug so nothing the middleware emits
// gets filtered out by the level gate.
func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// readOneLogLine reads exactly one JSON log line from the buffer and
// decodes it. Fails the test if zero or more than one line was emitted.
func readOneLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatalf("no log line emitted")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 log line, got %d:\n%s", len(lines), out)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("invalid JSON: %v\nline: %s", err, lines[0])
	}
	return rec
}

// runOnce spins up a router with only the log middleware and a noop
// handler, hits it once, and returns the captured buffer.
func runOnce(method, path string, opts ...func(*http.Request)) *bytes.Buffer {
	buf := &bytes.Buffer{}
	lg := newCapturingLogger(buf)
	mw := NewLogMiddleware(lg)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for _, o := range opts {
		o(req)
	}
	h.ServeHTTP(rr, req)
	return buf
}

// TestUnit_LogMiddleware_EmitsRequestLine verifies one JSON line is
// written per request and carries the canonical attribute set.
func TestUnit_LogMiddleware_EmitsRequestLine(t *testing.T) {
	t.Parallel()
	buf := runOnce(http.MethodGet, "/api/v1/me", func(r *http.Request) {
		r.RemoteAddr = "10.0.0.1:1234"
	})
	rec := readOneLogLine(t, buf)

	if got, want := rec["msg"], "http"; got != want {
		t.Errorf("msg = %v, want %v", got, want)
	}
	if got, want := rec["method"], "GET"; got != want {
		t.Errorf("method = %v, want %v", got, want)
	}
	if got, want := rec["path"], "/api/v1/me"; got != want {
		t.Errorf("path = %v, want %v", got, want)
	}
	// JSON numbers decode to float64 by default.
	if got, want := rec["status"], float64(200); got != want {
		t.Errorf("status = %v, want %v", got, want)
	}
	if _, ok := rec["dur"]; !ok {
		t.Errorf("dur attribute missing: %#v", rec)
	}
	if got, want := rec["remote"], "10.0.0.1:1234"; got != want {
		t.Errorf("remote = %v, want %v", got, want)
	}
	if _, ok := rec["user_sub"]; ok {
		t.Errorf("user_sub present for anonymous request: %#v", rec)
	}
}

// TestUnit_LogMiddleware_SkipsNoisyPaths pins the skip list so noisy
// probes / scrapes / asset fetches don't flood the log stream.
func TestUnit_LogMiddleware_SkipsNoisyPaths(t *testing.T) {
	t.Parallel()
	cases := []string{
		"/metrics",
		"/healthz",
		"/readyz",
		"/favicon.ico",
		"/static/styles.css",
		"/static/js/app.js",
		"/static/fonts/iosevka.woff2",
	}
	for _, path := range cases {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			buf := runOnce(http.MethodGet, path)
			if buf.Len() != 0 {
				t.Errorf("expected no log line for %s, got:\n%s", path, buf.String())
			}
		})
	}
}

// TestUnit_LogMiddleware_LogsApplicationPaths complements the skip-list
// test: paths NOT on the skip list must still produce one line. Guards
// against an over-eager prefix match (e.g. /staticfoo matching /static).
func TestUnit_LogMiddleware_LogsApplicationPaths(t *testing.T) {
	t.Parallel()
	cases := []string{
		"/",
		"/login",
		"/api/v1/me",
		"/worktime",
		"/staticfoo", // NOT under /static/
	}
	for _, path := range cases {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			buf := runOnce(http.MethodGet, path)
			if buf.Len() == 0 {
				t.Errorf("expected log line for %s, got nothing", path)
			}
		})
	}
}

// TestUnit_LogMiddleware_IncludesUserSub injects an authenticated user
// into the request context (mirroring NewBearerMiddleware's WithUser
// call) and asserts user_sub appears on the log line.
func TestUnit_LogMiddleware_IncludesUserSub(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	lg := newCapturingLogger(buf)
	mw := NewLogMiddleware(lg)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	u := domain.User{OIDCSub: "auth0|abc123"}
	req = req.WithContext(WithUser(req.Context(), u))
	h.ServeHTTP(rr, req)

	rec := readOneLogLine(t, buf)
	if got, want := rec["user_sub"], "auth0|abc123"; got != want {
		t.Errorf("user_sub = %v, want %v", got, want)
	}
}

// TestUnit_LogMiddleware_UsesProvidedLogger ensures the middleware
// honours the logger passed at construction time rather than silently
// dropping output into slog.Default(). Sets a sentinel default logger
// so we can detect leakage.
func TestUnit_LogMiddleware_UsesProvidedLogger(t *testing.T) {
	// Not Parallel — mutates slog.Default(); avoid stepping on the
	// nil-fallback test below.
	provided := &bytes.Buffer{}
	leaked := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(newCapturingLogger(leaked))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mw := NewLogMiddleware(newCapturingLogger(provided))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	h.ServeHTTP(rr, req)

	if provided.Len() == 0 {
		t.Errorf("provided logger received nothing; default got:\n%s", leaked.String())
	}
	if leaked.Len() != 0 {
		t.Errorf("default logger leaked output:\n%s", leaked.String())
	}
}

// TestUnit_LogMiddleware_NilLoggerFallsBackToDefault verifies the
// nil-tolerance contract — wiring NewLogMiddleware(nil) must not panic
// and must instead route through slog.Default().
func TestUnit_LogMiddleware_NilLoggerFallsBackToDefault(t *testing.T) {
	// Not Parallel — mutates slog.Default().
	captured := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(newCapturingLogger(captured))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mw := NewLogMiddleware(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	h.ServeHTTP(rr, req)

	if captured.Len() == 0 {
		t.Errorf("nil-logger fallback emitted nothing; expected slog.Default() to receive the line")
	}
}

// TestUnit_LogMiddleware_CapturesNon200Status ensures the statusRecorder
// reuse from metrics.go survives the second wrap (Metrics → Logging) so
// the log line reports the actual handler verdict, not 200.
func TestUnit_LogMiddleware_CapturesNon200Status(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	lg := newCapturingLogger(buf)
	mw := NewLogMiddleware(lg)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	h.ServeHTTP(rr, req)

	rec := readOneLogLine(t, buf)
	if got, want := rec["status"], float64(http.StatusForbidden); got != want {
		t.Errorf("status = %v, want %v", got, want)
	}
}

// TestUnit_ShouldSkipLog_PrefixBoundary pins the /static/ prefix
// behaviour as a unit test — guards against a regression that drops
// the trailing slash and starts skipping /staticfoo too.
func TestUnit_ShouldSkipLog_PrefixBoundary(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"/static/":         true,
		"/static/foo":      true,
		"/static":          false,
		"/staticfoo":       false,
		"/healthz":         true,
		"/healthzfoo":      false,
		"/metrics":         true,
		"/metrics/extra":   false,
		"/readyz":          true,
		"/favicon.ico":     true,
		"/favicon.ico.map": false,
		"/":                false,
		"/api/v1/sessions": false,
	}
	for path, want := range cases {
		if got := shouldSkipLog(path); got != want {
			t.Errorf("shouldSkipLog(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestUnit_LogMiddleware_PropagatesRequestContext is a sanity check —
// LogAttrs receives the request context (so any context tracer / span
// IDs added downstream propagate). We assert that by passing a
// custom-value context and reading it back from a Handler-injected
// sentinel; if the middleware swallowed the context the value would
// not be visible to the inner handler.
func TestUnit_LogMiddleware_PropagatesRequestContext(t *testing.T) {
	t.Parallel()
	type sentinelKey struct{}
	want := "spanX"
	buf := &bytes.Buffer{}
	lg := newCapturingLogger(buf)

	var seenInHandler any
	mw := NewLogMiddleware(lg)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInHandler = r.Context().Value(sentinelKey{})
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), sentinelKey{}, want))
	h.ServeHTTP(rr, req)

	if seenInHandler != want {
		t.Errorf("inner handler ctx value = %v, want %v", seenInHandler, want)
	}
}
