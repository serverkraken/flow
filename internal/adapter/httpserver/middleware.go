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

// ctxWithUser stores the authenticated user and the only actor provenance the
// current auth architecture can prove: the verified human principal. Request
// headers and MCP ClientInfo are caller-controlled metadata, not audit identity.
func ctxWithUser(ctx context.Context, u domain.User) context.Context {
	ctx = context.WithValue(ctx, userKey, u)
	ref := strings.TrimSpace(u.DisplayName)
	if ref == "" {
		ref = strings.TrimSpace(u.Username)
	}
	if ref == "" {
		ref = u.ID
	}
	return actor.WithContext(ctx, actor.AuthenticatedHuman(ref))
}

// resolveBearer verifies a bearer token and ensures the user. ok=false on any
// failure (used by authAny, which then tries the cookie). machine=true reports
// a VERIFIED machine token, which authAny must answer with 403 rather than let
// fall through — see authAny.
func (s *Server) resolveBearer(r *http.Request) (u domain.User, machine bool, ok bool) {
	raw := bearerToken(r)
	if raw == "" {
		return domain.User{}, false, false
	}
	id, err := s.Verifier.Verify(r.Context(), raw)
	if err != nil {
		return domain.User{}, false, false
	}
	if id.Machine {
		return domain.User{}, true, false
	}
	u, err = s.Ensure.Execute(r.Context(), id)
	if err != nil {
		return domain.User{}, false, false
	}
	return u, false, true
}

// bearerToken extracts the raw token, or "" when the header is absent or not a
// Bearer header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	raw := strings.TrimPrefix(h, "Bearer ")
	if raw == h {
		return ""
	}
	return raw
}

// auth verifies the bearer token, ensures the user, and stores it in context.
// Machine tokens are refused: a route accepts them only by being wrapped in
// authMachineOK, so a newly added route is machine-tight without anyone having
// to remember it.
func (s *Server) auth(next http.Handler) http.Handler {
	return s.authWith(next, false)
}

// authMachineOK is auth plus machine credentials. Wrap only routes a headless
// client legitimately needs.
func (s *Server) authMachineOK(next http.Handler) http.Handler {
	return s.authWith(next, true)
}

func (s *Server) authWith(next http.Handler, allowMachine bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		id, err := s.Verifier.Verify(r.Context(), raw)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if id.Machine {
			if !allowMachine {
				http.Error(w, "machine tokens are not accepted on this route", http.StatusForbidden)
				return
			}
			u, label, err := s.resolveMachine(r.Context(), id)
			if err != nil {
				if errors.Is(err, errMachineStore) {
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctxWithMachine(r.Context(), u, label)))
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
		next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
	})
}
