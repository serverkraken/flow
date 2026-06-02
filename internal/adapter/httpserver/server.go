package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Server wires the HTTP handlers into a Chi router. Construction is
// deliberately small — every endpoint group lives in its own file so the
// router stays a wiring index, never a god-object.
type Server struct {
	router chi.Router
}

// New constructs the server with the supplied readiness check. The check is
// run on every /readyz request; pass `func() error { return nil }` when no
// dependencies have been wired up yet.
func New(readyCheck ReadinessCheck) *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(readyCheck))
	return &Server{router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
