package httpserver

import (
	"net/http"
)

// ReadinessCheck returns nil when the dependency is ready (e.g. DB ping
// succeeds, JWKS cache primed). Anything else means /readyz reports 503.
type ReadinessCheck func() error

// NewReadyzHandler returns a readiness probe. It runs the injected check
// on every request — wrap in caching if the check is expensive.
func NewReadyzHandler(check ReadinessCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := check(); err != nil {
			http.Error(w, "not ready: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
}
