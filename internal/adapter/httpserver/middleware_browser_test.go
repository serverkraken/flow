package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// errFake is a sentinel decode error used to simulate an expired/invalid cookie.
var errFake = errors.New("fake decode error")

// TestUnit_BrowserMiddleware_SSEAndHTMX_Returns401 verifies that unauthenticated
// machine-style requests (EventSource, HTMX partials) receive 401 instead of a
// 302 redirect to the landing page — a redirect silently kills an SSE stream.
func TestUnit_BrowserMiddleware_SSEAndHTMX_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBrowserAuthMiddleware(stubSess{decodeErr: nil}, "flow_session", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	cases := []struct {
		name   string
		header http.Header
	}{
		{"sse", http.Header{"Accept": []string{"text/event-stream"}}},
		{"htmx", http.Header{"Hx-Request": []string{"true"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
			for k, v := range tc.header {
				req.Header[k] = v
			}
			// No cookie → unauthenticated.
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401", tc.name, rec.Code)
			}
		})
	}
}

// TestUnit_BrowserMiddleware_SSEAndHTMX_ExpiredCookie_Returns401 verifies that an
// invalid/expired cookie also yields 401 (not 302) for machine-style clients.
func TestUnit_BrowserMiddleware_SSEAndHTMX_ExpiredCookie_Returns401(t *testing.T) {
	t.Parallel()
	// stubSess with a decode error simulates an expired/invalid cookie.
	mw := NewBrowserAuthMiddleware(stubSess{decodeErr: errFake}, "flow_session", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	cases := []struct {
		name   string
		header http.Header
	}{
		{"sse-expired", http.Header{"Accept": []string{"text/event-stream"}}},
		{"htmx-expired", http.Header{"Hx-Request": []string{"true"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
			req.AddCookie(&http.Cookie{Name: "flow_session", Value: "stale-value"})
			for k, v := range tc.header {
				req.Header[k] = v
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401", tc.name, rec.Code)
			}
		})
	}
}

// TestUnit_BrowserMiddleware_HTMLRequest_Returns302 preserves the original
// behaviour: a plain browser GET without SSE/HTMX headers still 302-redirects
// to LandingPath on missing cookie.
func TestUnit_BrowserMiddleware_HTMLRequest_Returns302(t *testing.T) {
	t.Parallel()
	mw := NewBrowserAuthMiddleware(stubSess{}, "flow_session", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worktime", nil)
	// No cookie, no special headers — plain browser navigation.
	mw(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != LandingPath {
		t.Errorf("Location = %q, want %q", loc, LandingPath)
	}
}
