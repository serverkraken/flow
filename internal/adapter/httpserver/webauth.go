package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

// Authenticator is the OIDC auth-code-flow port (oidcauth.Authenticator).
type Authenticator interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (ports.Identity, error)
}

// SessionCodec issues/parses the signed browser session cookie value.
type SessionCodec interface {
	Issue(userID string) (string, error)
	Parse(token string) (string, error)
}

// authAny accepts either a bearer token (TUI) or a session cookie (browser).
// Fully implemented in Task 5; for now it falls back to bearer-only.
func (s *Server) authAny(next http.Handler) http.Handler { return s.auth(next) }

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
