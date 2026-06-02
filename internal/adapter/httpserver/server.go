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

func New() *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	return &Server{router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
