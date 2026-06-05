package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

// LandingPath is where the browser middleware sends unauthenticated users.
// Kept here (rather than in webui) so the middleware and the handler agree
// on the URL without importing each other.
const LandingPath = "/auth/landing"

// NewBrowserAuthMiddleware mirrors NewAuthMiddleware but on a missing or
// invalid cookie it 302-redirects to LandingPath instead of returning 401 —
// the WebUI variant for human-browser traffic where a JSON 401 would be
// useless. When users is non-nil it also EnsureBySubs the user and injects
// the resolved domain.User into context, matching the bearer middleware.
//
// The redirect intentionally skips paths that already render an HTML response
// for the unauthenticated case (LandingPath itself, /login, /auth/callback),
// avoiding redirect loops. The HTTP layer (router) is responsible for not
// wrapping those routes with this middleware in the first place; the
// internal guard is defence-in-depth.
func NewBrowserAuthMiddleware(
	sess ports.BrowserSessionStore,
	cookieName string,
	users ports.UserStore,
) func(http.Handler) http.Handler {
	cache := &userCache{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == LandingPath || r.URL.Path == "/login" || r.URL.Path == "/auth/callback" {
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				http.Redirect(w, r, LandingPath, http.StatusFound)
				return
			}
			var sv sessionValue
			if err := sess.Decode(cookieName, c.Value, &sv); err != nil {
				http.Redirect(w, r, LandingPath, http.StatusFound)
				return
			}
			ctx := WithSub(r.Context(), sv)
			if users != nil {
				u, ok := ensureUser(w, r, users, cache, sv.Sub, sv.Email, sv.Name)
				if !ok {
					return
				}
				ctx = WithUser(ctx, u)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionValueFromContext exposes the cookie session value for the WebUI
// handlers. Returns the empty value and false when no session is attached
// (i.e. unauthenticated request that slipped past the middleware).
func SessionValueFromContext(ctx interface {
	Value(any) any
}) (sub, email, name string, ok bool) {
	sv, ok := ctx.Value(ctxKeySession).(sessionValue)
	if !ok {
		return "", "", "", false
	}
	return sv.Sub, sv.Email, sv.Name, true
}
