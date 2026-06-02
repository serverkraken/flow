package httpserver

import (
	"context"
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
