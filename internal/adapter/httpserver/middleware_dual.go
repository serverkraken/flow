// Package httpserver implements the REST and bearer APIs.
package httpserver

import "net/http"

// NewBearerOrCookieMiddleware lets ONE route serve both client classes
// (Spec §5: SSE für Browser UND TUI/MCP): requests carrying an
// Authorization header take the bearer path, everything else the
// browser-cookie path. Both wrapped middlewares already put the resolved
// domain.User into the context, which is all /api/v1/events needs.
func NewBearerOrCookieMiddleware(bearer, cookie func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		bearerChain := bearer(next)
		cookieChain := cookie(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				bearerChain.ServeHTTP(w, r)
				return
			}
			cookieChain.ServeHTTP(w, r)
		})
	}
}
