package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

// TestMachineRouteMatrix pins the opt-in list from the design spec §5.1. A
// route reaching the handler answers something other than 403; a refused route
// answers exactly 403 with the route message. The point is the ALLOW/DENY
// split, not each handler's own status.
func TestMachineRouteMatrix(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	mux := srv.Routes()

	cases := []struct {
		method, path string
		allowed      bool
	}{
		{http.MethodPost, "/api/v1/documents", true},
		{http.MethodGet, "/api/v1/documents", true},
		{http.MethodGet, "/api/v1/documents/doc-1", true},
		{http.MethodPut, "/api/v1/documents/doc-1", true},
		{http.MethodPatch, "/api/v1/documents/doc-1", true},
		{http.MethodGet, "/api/v1/me", true},

		{http.MethodDelete, "/api/v1/documents/doc-1", false},
		{http.MethodPost, "/api/v1/documents/import", false},
		{http.MethodPut, "/api/v1/documents/by-path", false},
		{http.MethodPost, "/api/v1/documents/doc-1/pin", false},
		{http.MethodPost, "/api/v1/documents/doc-1/archive", false},
		{http.MethodPost, "/api/v1/documents/doc-1/move", false},
		{http.MethodPost, "/api/v1/documents/doc-1/context-mode", false},
		{http.MethodPost, "/api/v1/nodes", false},
		{http.MethodGet, "/api/v1/nodes", false},
		{http.MethodPost, "/api/v1/sessions", false},
		{http.MethodGet, "/api/v1/settings", false},
		{http.MethodPost, "/api/v1/ics-token/regenerate", false},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			reached, code, body := reachedHandler(mux, tc.method, tc.path)
			if reached != tc.allowed {
				t.Fatalf("reached handler = %v, want %v (status %d, body %q)",
					reached, tc.allowed, code, body)
			}
		})
	}
}

// reachedHandler reports whether a machine-token request got PAST the auth
// middleware.
//
// machineTestServer wires no document usecases, so an allowed route runs into
// a zero-valued usecase and may panic inside its handler. That panic IS the
// signal this test wants — the request demonstrably left the middleware — and
// is recovered here rather than papered over by wiring a full set of fakes,
// which would test the handlers rather than the opt-in list.
func reachedHandler(mux http.Handler, method, path string) (reached bool, code int, body string) {
	defer func() {
		if recover() != nil {
			reached = true
		}
	}()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	code, body = rec.Code, rec.Body.String()
	reached = !(code == http.StatusForbidden &&
		strings.Contains(body, "machine tokens are not accepted on this route"))
	return
}
