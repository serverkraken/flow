package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// a throwaway client value; the manager never dereferences it in these tests
// because fn is a fake that ignores the client.
func dummyClient() *apiclient.Client { return apiclient.New("http://example", "tok") }

func TestAuthManager_BuildsOnceAndFiresOnAuthOnce(t *testing.T) {
	builds, auths := 0, 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { builds++; return dummyClient(), nil },
		func(context.Context, *apiclient.Client) { auths++ },
	)
	for i := 0; i < 3; i++ {
		if _, err := m.client(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1 (cached)", builds)
	}
	if auths != 1 {
		t.Fatalf("onAuth fired %d times, want exactly 1", auths)
	}
}

func TestAuthManager_NoTokenReturnsLoginRequired_NoOnAuth(t *testing.T) {
	auths := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return nil, clientauth.ErrNotLoggedIn },
		func(context.Context, *apiclient.Client) { auths++ },
	)
	_, err := m.client(context.Background())
	if !errors.Is(err, errLoginRequired) {
		t.Fatalf("err = %v, want errLoginRequired", err)
	}
	if auths != 0 {
		t.Fatal("onAuth must not fire when auth never succeeded")
	}
}

func TestAuthManager_TransientBuildErrorIsNotLogout(t *testing.T) {
	sentinel := errors.New("token lock timed out")
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return nil, sentinel },
		nil,
	)
	_, err := m.client(context.Background())
	if !errors.Is(err, sentinel) || errors.Is(err, errLoginRequired) {
		t.Fatalf("err = %v, want transient build error", err)
	}
}

func TestAuthManager_Do_RetriesOnceOn401ThenSucceeds(t *testing.T) {
	builds, auths := 0, 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { builds++; return dummyClient(), nil },
		func(context.Context, *apiclient.Client) { auths++ },
	)
	calls := 0
	err := m.Do(context.Background(), func(*apiclient.Client) error {
		calls++
		if calls == 1 {
			return &apiclient.APIError{StatusCode: http.StatusUnauthorized}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do = %v, want nil after one retry", err)
	}
	if calls != 2 {
		t.Fatalf("fn called %d times, want 2 (call + one retry)", calls)
	}
	if builds != 2 {
		t.Fatalf("builds = %d, want 2 (initial + rebuild after reset)", builds)
	}
	if auths != 2 {
		t.Fatalf("onAuth fired %d times, want 2 so resources reconcile after identity rebuild", auths)
	}
}

func TestAuthManager_Do_NonAuthErrorNotRetried(t *testing.T) {
	builds := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { builds++; return dummyClient(), nil },
		func(context.Context, *apiclient.Client) {},
	)
	calls := 0
	sentinel := errors.New("network down")
	err := m.Do(context.Background(), func(*apiclient.Client) error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do = %v, want the network error propagated", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1 (no retry on non-auth error)", calls)
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1 (no rebuild on non-auth error)", builds)
	}
}

func TestAuthManager_Do_PersistentAuthErrorBecomesLoginRequired(t *testing.T) {
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return dummyClient(), nil },
		func(context.Context, *apiclient.Client) {},
	)
	calls := 0
	err := m.Do(context.Background(), func(*apiclient.Client) error {
		calls++
		return &apiclient.APIError{StatusCode: http.StatusUnauthorized}
	})
	if !errors.Is(err, errLoginRequired) {
		t.Fatalf("Do = %v, want errLoginRequired after retry also 401", err)
	}
	if calls != 2 {
		t.Fatalf("fn called %d times, want 2 (call + one retry)", calls)
	}
}

func TestAuthManager_Do_RecoversAfterReLogin(t *testing.T) {
	// First build has no token (logged out); a later build succeeds (post flow login).
	var mu sync.Mutex
	loggedIn := false
	auths := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) {
			mu.Lock()
			defer mu.Unlock()
			if !loggedIn {
				return nil, clientauth.ErrNotLoggedIn
			}
			return dummyClient(), nil
		},
		func(context.Context, *apiclient.Client) { auths++ },
	)
	// logged out → login required, onAuth never ran
	if err := m.Do(context.Background(), func(*apiclient.Client) error { return nil }); !errors.Is(err, errLoginRequired) {
		t.Fatalf("logged-out Do = %v, want errLoginRequired", err)
	}
	// user runs `flow login`
	mu.Lock()
	loggedIn = true
	mu.Unlock()
	// next call recovers WITHOUT any reconnect; onAuth fires now (first success)
	if err := m.Do(context.Background(), func(*apiclient.Client) error { return nil }); err != nil {
		t.Fatalf("post-login Do = %v, want nil", err)
	}
	if auths != 1 {
		t.Fatalf("onAuth fired %d times, want 1 (on first success after re-login)", auths)
	}
}

// TestAuthManager_BuildUsesProcessContextNotRequest guards the original
// "oidcdevice: context canceled" wedge. Client construction and initial store
// loading use a process-lifetime context; the coordinated auth transport later
// overlays each HTTP request's shorter deadline for refresh work.
func TestAuthManager_BuildUsesProcessContextNotRequest(t *testing.T) {
	var gotBuildCtx context.Context
	m := newAuthManager(
		func(c context.Context) (*apiclient.Client, error) { gotBuildCtx = c; return dummyClient(), nil },
		nil,
	)
	reqCtx, cancel := context.WithCancel(context.Background())
	if _, err := m.client(reqCtx); err != nil {
		t.Fatal(err)
	}
	cancel() // the triggering request ends

	if gotBuildCtx == reqCtx {
		t.Fatal("build used the request ctx; the cached token source is poisoned once the request ends")
	}
	if gotBuildCtx == nil || gotBuildCtx.Err() != nil {
		t.Fatalf("build ctx must stay live for the process; got err=%v — future token refreshes would fail 'context canceled'", func() error {
			if gotBuildCtx == nil {
				return errors.New("nil")
			}
			return gotBuildCtx.Err()
		}())
	}
}

func TestIsAuthError(t *testing.T) {
	if !isAuthError(&apiclient.APIError{StatusCode: http.StatusUnauthorized}) {
		t.Error("401 is an auth error")
	}
	if !isAuthError(clientauth.ErrNotLoggedIn) {
		t.Error("ErrNotLoggedIn is an auth error")
	}
	if isAuthError(errors.New("dial tcp: connection refused")) {
		t.Error("network error is NOT an auth error")
	}
	if isAuthError(&apiclient.APIError{StatusCode: http.StatusInternalServerError}) {
		t.Error("500 is NOT an auth error")
	}
}

func TestIsAuthError_TransientProviderErrorIsNotLogout(t *testing.T) {
	if isAuthError(fmt.Errorf("refresh: temporarily unavailable")) {
		t.Error("transient provider error is not an auth error")
	}
}
