package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Server_New_HealthzAlwaysOK(t *testing.T) {
	t.Parallel()
	s := New(func() error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", rr.Code)
	}
}

func TestUnit_Server_New_ReadyzReflectsCheck(t *testing.T) {
	t.Parallel()

	// Ready
	sOK := New(func() error { return nil })
	rr := httptest.NewRecorder()
	sOK.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("ready check OK: status = %d, want 200", rr.Code)
	}

	// Not ready
	sErr := New(func() error { return errors.New("db unreachable") })
	rrErr := httptest.NewRecorder()
	sErr.Handler().ServeHTTP(rrErr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rrErr.Code == http.StatusOK {
		t.Errorf("ready check failing: status = %d, want non-200", rrErr.Code)
	}
}

func TestUnit_Server_SetBaseURL_PersistsValue(t *testing.T) {
	t.Parallel()
	s := New(func() error { return nil })
	s.SetBaseURL("https://flow.example.com")
	if s.baseURL != "https://flow.example.com" {
		t.Errorf("baseURL = %q, want %q", s.baseURL, "https://flow.example.com")
	}
}

func TestUnit_Server_Handler_NonNil(t *testing.T) {
	t.Parallel()
	s := New(func() error { return nil })
	if s.Handler() == nil {
		t.Error("Handler() returned nil")
	}
}
