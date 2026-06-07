package httpserver

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

type ctxKey int

const ctxKeySession ctxKey = 1

// WithSub attaches the resolved session value to a context. Used by handlers
// downstream of NewAuthMiddleware / NewBearerMiddleware.
func WithSub(ctx context.Context, sv sessionValue) context.Context {
	return context.WithValue(ctx, ctxKeySession, sv)
}

// SubFromContext returns the sub claim of the authenticated user, empty
// string if not present.
func SubFromContext(ctx context.Context) string {
	sv, _ := ctx.Value(ctxKeySession).(sessionValue)
	return sv.Sub
}

// sessionFromContext returns the full sessionValue.
func sessionFromContext(ctx context.Context) (sessionValue, bool) {
	sv, ok := ctx.Value(ctxKeySession).(sessionValue)
	return sv, ok
}

// NewAuthMiddleware enforces a valid session cookie. Returns 401 if missing
// or invalid. Attaches the sessionValue to context on success.
func NewAuthMiddleware(sess ports.BrowserSessionStore, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var sv sessionValue
			if err := sess.Decode(cookieName, c.Value, &sv); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithSub(r.Context(), sv)))
		})
	}
}

// NewBearerMiddleware enforces Authorization: Bearer <jwt>. Verifies via
// AuthProvider, runs AccessChecker, attaches identity to context.
//
// When users is non-nil the middleware also calls users.EnsureBySub so that
// a User row always exists before the request reaches a handler; the resolved
// domain.User is injected via WithUser / UserFromContext. Pass nil to retain
// the original M1 behaviour (no DB call, no user in context).
func NewBearerMiddleware(prov ports.AuthProvider, access ports.AccessChecker, users ports.UserStore) func(http.Handler) http.Handler {
	cache := &userCache{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := h[len(prefix):]
			id, err := prov.Verify(r.Context(), token)
			if err != nil {
				// Surface the real verifier error (iss/aud/exp/signature) in
				// the server log so a bearer 401 is diagnosable without
				// decoding the JWT by hand; the client still receives only a
				// generic message. slog.Default() matches httpserver's
				// ambient-logger convention (see NewLogMiddleware).
				slog.WarnContext(r.Context(), "bearer token verification failed",
					slog.String("path", r.URL.Path),
					slog.Any("err", err))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !access.Allow(id) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			sv := sessionValue{Sub: id.Sub, Email: id.Email, Name: id.Name}
			ctx := WithSub(r.Context(), sv)
			if users != nil {
				u, ok := ensureUser(w, r, users, cache, id.Sub, id.Email, id.Name)
				if !ok {
					return
				}
				ctx = WithUser(ctx, u)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
