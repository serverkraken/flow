// internal/adapter/httpserver/middleware_dual_test.go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerOrCookie_RoutesByAuthorizationHeader(t *testing.T) {
	t.Parallel()
	mark := func(label string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Path", label)
				next.ServeHTTP(w, r)
			})
		}
	}
	mw := NewBearerOrCookieMiddleware(mark("bearer"), mark("cookie"))
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Path") != "bearer" {
		t.Errorf("with Authorization: want bearer path, got %q", rec.Header().Get("X-Path"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Header().Get("X-Path") != "cookie" {
		t.Errorf("without Authorization: want cookie path, got %q", rec.Header().Get("X-Path"))
	}
}
