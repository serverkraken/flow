package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Healthz_Returns200OK(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	NewHealthzHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want %q", rr.Body.String(), "ok\n")
	}
}
