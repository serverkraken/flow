package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnit_Readyz_AllChecksOK_Returns200(t *testing.T) {
	t.Parallel()
	h := NewReadyzHandler(func() error { return nil })

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "ready\n" {
		t.Fatalf("body = %q, want %q", rr.Body.String(), "ready\n")
	}
}

func TestUnit_Readyz_CheckFails_Returns503(t *testing.T) {
	t.Parallel()
	h := NewReadyzHandler(func() error { return errors.New("db down") })

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not ready: db down") {
		t.Fatalf("body = %q, want it to contain %q", rr.Body.String(), "not ready: db down")
	}
}
