package httpserver

import (
	"net/http"
)

// NewHealthzHandler returns a liveness probe. Always 200 while the process is
// up; readiness is separate (see /readyz). Used by Kubernetes / docker-compose
// healthchecks.
func NewHealthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}
