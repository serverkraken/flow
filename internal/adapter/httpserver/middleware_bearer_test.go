package httpserver

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_BearerMiddleware_NoHeader_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true}, nil)
	rr := httptest.NewRecorder()
	mw(http.NotFoundHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_BearerMiddleware_WrongScheme_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true}, nil)
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
	mw := NewBearerMiddleware(stubProv{err: errors.New("bad jwt")}, stubAccessAll{allow: true}, nil)
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
	mw := NewBearerMiddleware(stubProv{id: ports.Identity{Sub: "u-1"}}, stubAccessAll{allow: false}, nil)
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
	mw := NewBearerMiddleware(stubProv{id: ports.Identity{Sub: "u-1", Email: "u@x"}}, stubAccessAll{allow: true}, nil)
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

func TestUnit_BearerMiddleware_VerifyFails_LogsUnderlyingError(t *testing.T) {
	// Not Parallel — mutates slog.Default().
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var buf bytes.Buffer
	slog.SetDefault(newCapturingLogger(&buf))

	mw := NewBearerMiddleware(stubProv{err: errors.New("verify: iss did not match")}, stubAccessAll{allow: true}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me-bearer", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run when verify fails")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	// The generic client body stays generic; the diagnostic lives in the log.
	if body := strings.TrimSpace(rr.Body.String()); body != "unauthorized" {
		t.Errorf("client body = %q, want generic \"unauthorized\"", body)
	}
	got := buf.String()
	for _, want := range []string{"bearer token verification failed", "iss did not match", "/api/v1/me-bearer"} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q; got: %s", want, got)
		}
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
