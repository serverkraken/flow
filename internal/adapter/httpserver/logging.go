package httpserver

// logging.go — Plan F · Task 7.
//
// Structured JSON request-log middleware. One log line per request, written
// to the supplied *slog.Logger. Reuses statusRecorder from metrics.go so
// the status seen here matches what the metrics middleware counted.
//
// Middleware ordering in NewWithAuth (outer→inner):
//
//   Metrics  → records {method,route,status} + duration on EVERY request
//   Logging  → emits one "http" log line per request, post-handler
//   Auth     → enforces session / bearer; the logging middleware sits ABOVE
//              auth so its status field reflects 401/403 verdicts too
//
// Noisy paths are skipped to keep the log stream signal-rich:
//   - /metrics  (Prometheus scrapes every 15s; not interesting)
//   - /healthz  (liveness probe; pings every few seconds)
//   - /readyz   (readiness probe; pings every few seconds)
//   - /favicon.ico (browser noise on every page load)
//   - /static/* (asset fetches; flood the log on first paint)
//
// When the request is authenticated the user_sub attribute is added so
// audit / correlation works without joining against the session store.

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NewLogMiddleware returns a middleware that emits one structured JSON
// log line per request. If logger is nil it falls back to slog.Default()
// so the middleware is safe to wire even when AuthDeps.Logger was left
// unset (matches the nil-tolerance convention used elsewhere in this
// package).
func NewLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			if shouldSkipLog(r.URL.Path) {
				return
			}

			lg := logger
			if lg == nil {
				lg = slog.Default()
			}

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("dur", time.Since(start)),
				slog.String("remote", r.RemoteAddr),
			}
			if user, ok := UserFromContext(r.Context()); ok {
				attrs = append(attrs, slog.String("user_sub", user.OIDCSub))
			}
			lg.LogAttrs(r.Context(), slog.LevelInfo, "http", attrs...)
		})
	}
}

// shouldSkipLog returns true for noisy paths that would otherwise flood
// the log with prometheus scrapes, kubelet probes, and asset fetches.
// Kept as a free function so logging_test.go can pin the skip list
// without reaching into middleware closure state.
func shouldSkipLog(p string) bool {
	switch p {
	case "/metrics", "/healthz", "/readyz", "/favicon.ico":
		return true
	}
	return strings.HasPrefix(p, "/static/")
}
