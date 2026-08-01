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
		// Static siblings of the newly machine-opened GET /documents/{id}
		// wildcard. They stay s.auth-only; this pins that the wildcard cannot
		// swallow them. Go's ServeMux has preferred the more specific pattern
		// regardless of registration order since 1.22, but that's a routing
		// guarantee, not a test — a router swap or reorder could silently
		// hand these to handleGetDocument under authMachineOK otherwise.
		{http.MethodGet, "/api/v1/documents/tags", false},
		{http.MethodGet, "/api/v1/documents/archived", false},
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
//
// The recover is deliberately not scoped to "inside the handler only" — it
// catches any panic during ServeHTTP, including one raised inside the
// middleware chain itself before the authorization decision is made. That
// would misreport an unauthorized request as "reached", but it isn't live
// today: machineTestServer wires a deterministic FakeVerifier and a
// populated FakeUserStore, so authMachineOK's resolveMachine step always
// completes cleanly before next.ServeHTTP runs. If that ever stops holding
// (e.g. a test fixture starts returning verifier/store errors), this helper
// would need to distinguish where the panic originated.
//
// Only two of the six ALLOW cases actually exercise the panic path: GET
// /api/v1/documents and GET /api/v1/documents/{id}. GET /api/v1/me returns a
// real 200 (handleMe doesn't touch the nil document usecases), and the
// POST/PUT/PATCH document routes return 400 from decodeJSONBody on the empty
// test body before ever reaching a nil usecase — reached is still true for
// those because a 400 isn't the machine-refusal 403.
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
