package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Server wires the HTTP handlers into a Chi router. Construction is
// deliberately small — every endpoint group lives in its own file so the
// router stays a wiring index, never a god-object.
type Server struct {
	router  chi.Router
	baseURL string
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

// NewWithAuth assembles the Phase-1-M1 server: healthz + readyz + browser
// auth (login/callback/logout). Plain New() is kept for tests that don't
// need auth.
func NewWithAuth(d AuthDeps) *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(d.Ready))

	ab := newAuthBrowser(d)
	r.Get("/login", ab.handleLogin)
	r.Get("/auth/callback", ab.handleCallback)
	r.Get("/logout", ab.handleLogout)

	return &Server{router: r, baseURL: d.BaseURL}
}

// SetBaseURL allows tests to swap baseURL after the server is constructed
// (so RedirectURL matches httptest's random port).
func (s *Server) SetBaseURL(u string) { s.baseURL = u }
