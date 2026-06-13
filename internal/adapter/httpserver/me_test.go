package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Me_WithContextSub_ReturnsJSON(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req = req.WithContext(WithSub(context.Background(), sessionValue{
		Sub: "user-1", Email: "alice@example.com", Name: "Alice",
	}))

	NewMeHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["sub"] != "user-1" {
		t.Errorf("sub = %v, want user-1", out["sub"])
	}
	if out["email"] != "alice@example.com" {
		t.Errorf("email = %v, want alice@example.com", out["email"])
	}
	if out["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", out["name"])
	}
}

func TestUnit_Me_NoContextSub_Returns401(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	NewMeHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
