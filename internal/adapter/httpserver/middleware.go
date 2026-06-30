package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type ctxKey int

const userKey ctxKey = 0

func userFrom(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey).(domain.User)
	return u, ok
}

// ctxWithUser stores the authenticated user AND the derived actor in ctx.
// The actor comes from the X-Flow-Actor header (set by the MCP client) or
// defaults to the human user.
func ctxWithUser(r *http.Request, u domain.User) context.Context {
	ctx := context.WithValue(r.Context(), userKey, u)
	return actor.WithContext(ctx, actor.FromHeader(r.Header.Get("X-Flow-Actor"), u.DisplayName))
}

// resolveBearer verifies a bearer token and ensures the user. Returns
// ok=false on any failure (used by authAny, which then tries the cookie).
func (s *Server) resolveBearer(r *http.Request) (domain.User, bool) {
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if raw == "" || raw == r.Header.Get("Authorization") {
		return domain.User{}, false
	}
	id, err := s.Verifier.Verify(r.Context(), raw)
	if err != nil {
		return domain.User{}, false
	}
	u, err := s.Ensure.Execute(r.Context(), id)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

// auth verifies the bearer token, ensures the user, and stores it in context.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" || raw == r.Header.Get("Authorization") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		id, err := s.Verifier.Verify(r.Context(), raw)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		u, err := s.Ensure.Execute(r.Context(), id)
		if errors.Is(err, usecase.ErrNotAllowed) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctxWithUser(r, u)))
	})
}
