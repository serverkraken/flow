package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// stubUserStore is a controllable UserStore for bearer middleware tests.
type stubUserStore struct {
	calls atomic.Int64
	user  domain.User
	err   error
}

func (s *stubUserStore) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	s.calls.Add(1)
	if s.err != nil {
		return domain.User{}, s.err
	}
	u := s.user
	u.OIDCSub = sub
	u.Email = email
	u.DisplayName = displayName
	return u, nil
}

func (s *stubUserStore) GetByID(_ string) (domain.User, error) {
	return domain.User{}, ports.ErrUserNotFound
}

func (s *stubUserStore) GetBySub(_ string) (domain.User, error) {
	return domain.User{}, ports.ErrUserNotFound
}

func bearerReq(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// TestUnit_BearerMiddleware_UsersNil_NoEnsureCall verifies that when users is
// nil the middleware behaves exactly as M1: session in ctx, no user in ctx.
func TestUnit_BearerMiddleware_UsersNil_NoEnsureCall(t *testing.T) {
	t.Parallel()
	prov := stubProv{id: ports.Identity{Sub: "u-nil", Email: "nil@x"}}
	mw := NewBearerMiddleware(prov, stubAccessAll{allow: true}, nil)

	var gotSub string
	var hasUser bool
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotSub = SubFromContext(r.Context())
		_, hasUser = UserFromContext(r.Context())
	})

	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, bearerReq("tok"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if gotSub != "u-nil" {
		t.Errorf("sub = %q, want u-nil", gotSub)
	}
	if hasUser {
		t.Error("UserFromContext returned true with nil UserStore, want false")
	}
}

// TestUnit_BearerMiddleware_FirstRequest_EnsureCalledOnce verifies that the
// first request triggers EnsureBySub and the resolved user is in context.
func TestUnit_BearerMiddleware_FirstRequest_EnsureCalledOnce(t *testing.T) {
	t.Parallel()
	store := &stubUserStore{user: domain.User{ID: "id-1"}}
	prov := stubProv{id: ports.Identity{Sub: "u-1", Email: "one@x", Name: "One"}}
	mw := NewBearerMiddleware(prov, stubAccessAll{allow: true}, store)

	var gotUser domain.User
	var hasUser bool
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUser, hasUser = UserFromContext(r.Context())
	})

	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, bearerReq("tok"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !hasUser {
		t.Fatal("UserFromContext returned false, want user in context")
	}
	if gotUser.OIDCSub != "u-1" {
		t.Errorf("user.OIDCSub = %q, want u-1", gotUser.OIDCSub)
	}
	if store.calls.Load() != 1 {
		t.Errorf("EnsureBySub calls = %d, want 1", store.calls.Load())
	}
}

// TestUnit_BearerMiddleware_SecondRequest_CacheHit verifies that a second
// request for the same sub does NOT call EnsureBySub again.
func TestUnit_BearerMiddleware_SecondRequest_CacheHit(t *testing.T) {
	t.Parallel()
	store := &stubUserStore{user: domain.User{ID: "id-2"}}
	prov := stubProv{id: ports.Identity{Sub: "u-2", Email: "two@x"}}
	mw := NewBearerMiddleware(prov, stubAccessAll{allow: true}, store)

	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

	// First request — populates cache.
	mw(handler).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))
	// Second request — must hit cache.
	var gotUser domain.User
	var hasUser bool
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUser, hasUser = UserFromContext(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))

	if !hasUser {
		t.Fatal("UserFromContext returned false on second request")
	}
	if gotUser.OIDCSub != "u-2" {
		t.Errorf("user.OIDCSub = %q, want u-2", gotUser.OIDCSub)
	}
	if store.calls.Load() != 1 {
		t.Errorf("EnsureBySub calls = %d after 2 requests, want 1 (cache hit)", store.calls.Load())
	}
}

// TestUnit_BearerMiddleware_TwoDifferentSubs_TwoCalls verifies that two
// distinct subs each trigger one EnsureBySub call and are cached independently.
func TestUnit_BearerMiddleware_TwoDifferentSubs_TwoCalls(t *testing.T) {
	t.Parallel()
	store := &stubUserStore{user: domain.User{ID: "id-multi"}}
	// We'll run two requests with different identities by building the
	// middleware once and swapping providers via separate middleware instances
	// that share no cache. To share the same cache, we use one middleware with
	// a provider that returns different subs per call via a slice.
	provA := stubProv{id: ports.Identity{Sub: "sub-a", Email: "a@x"}}
	provB := stubProv{id: ports.Identity{Sub: "sub-b", Email: "b@x"}}

	// Two middleware instances share the same store but have independent caches
	// (by design: one cache per middleware instance). Use same store to count
	// total calls.
	mwA := NewBearerMiddleware(provA, stubAccessAll{allow: true}, store)
	mwB := NewBearerMiddleware(provB, stubAccessAll{allow: true}, store)

	noop := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	mwA(noop).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))
	mwB(noop).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))

	// Each sub should have triggered exactly one call, two total.
	if store.calls.Load() != 2 {
		t.Errorf("EnsureBySub total calls = %d, want 2 (one per sub)", store.calls.Load())
	}

	// A second pass for each sub should produce no additional calls (cache hits).
	mwA(noop).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))
	mwB(noop).ServeHTTP(httptest.NewRecorder(), bearerReq("tok"))
	if store.calls.Load() != 2 {
		t.Errorf("EnsureBySub total calls = %d after cache hits, still want 2", store.calls.Load())
	}
}

// TestUnit_BearerMiddleware_EnsureError_Returns500 verifies that a store
// error surfaces as a 500 and the handler is not called.
func TestUnit_BearerMiddleware_EnsureError_Returns500(t *testing.T) {
	t.Parallel()
	store := &stubUserStore{err: errors.New("db unavailable")}
	prov := stubProv{id: ports.Identity{Sub: "u-err", Email: "err@x"}}
	mw := NewBearerMiddleware(prov, stubAccessAll{allow: true}, store)

	handlerCalled := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
	})

	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, bearerReq("tok"))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if handlerCalled {
		t.Error("downstream handler was called after store error, want early return")
	}
}
