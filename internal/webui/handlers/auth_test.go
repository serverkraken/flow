package handlers_test

// auth_test.go — Plan E · Task 15.
//
// Unit-tests the landing handler. The handler is otherwise only hit by
// scripts/smoke-m6-webui.sh; this gives `go test` a direct call to keep
// the func-level coverage off 0%.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/webui/handlers"
)

func TestLanding_RendersLoginTemplateWithIssuer(t *testing.T) {
	t.Parallel()
	h := handlers.NewLanding(handlers.AuthDeps{IssuerLabel: "dex (local-dev)"})

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/landing", nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html prefix", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control: got %q, want no-store", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "dex (local-dev)") {
		t.Errorf("body missing issuer label; body=%s", body)
	}
}
