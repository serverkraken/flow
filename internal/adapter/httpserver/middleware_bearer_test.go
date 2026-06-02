package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_BearerMiddleware_NoHeader_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true})
	rr := httptest.NewRecorder()
	mw(http.NotFoundHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_BearerMiddleware_WrongScheme_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_BearerMiddleware_VerifyFails_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{err: errors.New("bad jwt")}, stubAccessAll{allow: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_BearerMiddleware_ValidTokenAccessDenied_Returns403(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{id: ports.Identity{Sub: "u-1"}}, stubAccessAll{allow: false})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestUnit_BearerMiddleware_ValidToken_PassesThrough(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{id: ports.Identity{Sub: "u-1", Email: "u@x"}}, stubAccessAll{allow: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	got := ""
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = SubFromContext(r.Context())
	})).ServeHTTP(rr, req)
	if got != "u-1" {
		t.Errorf("sub = %q, want u-1", got)
	}
}

type stubProv struct {
	id  ports.Identity
	err error
}

func (s stubProv) Verify(_ context.Context, _ string) (ports.Identity, error) {
	if s.err != nil {
		return ports.Identity{}, s.err
	}
	return s.id, nil
}

type stubAccessAll struct{ allow bool }

func (s stubAccessAll) Allow(_ ports.Identity) bool { return s.allow }
