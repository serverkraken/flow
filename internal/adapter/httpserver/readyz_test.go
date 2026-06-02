package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
}

func TestUnit_Readyz_CheckFails_Returns503(t *testing.T) {
	t.Parallel()
	h := NewReadyzHandler(func() error { return errors.New("db down") })

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}
