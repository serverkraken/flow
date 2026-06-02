package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_AuthMiddleware_NoCookie_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSess{}, "flow_session")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_AuthMiddleware_EmptyCookie_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSess{}, "flow_session")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: ""})
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_AuthMiddleware_DecodeFails_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSess{decodeErr: errors.New("bad")}, "flow_session")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: "garbage"})
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_AuthMiddleware_ValidCookie_PassesThrough(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSess{sub: "user-1"}, "flow_session")
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := SubFromContext(r.Context()); got != "user-1" {
			t.Errorf("sub from context = %q, want user-1", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: "some-encoded-value"})
	mw(next).ServeHTTP(rr, req)
	if !called {
		t.Fatal("downstream handler was not called")
	}
}

// stubSess implements ports.BrowserSessionStore for middleware tests.
type stubSess struct {
	sub       string
	decodeErr error
}

func (s stubSess) Encode(_ string, _ any) (string, error) { return "", nil }
func (s stubSess) Decode(_ string, _ string, out any) error {
	if s.decodeErr != nil {
		return s.decodeErr
	}
	sv, ok := out.(*sessionValue)
	if !ok {
		return errors.New("stubSess: unexpected out type")
	}
	sv.Sub = s.sub
	return nil
}
