package httpsync

// White-box tests for runPull — internal package so we can call unexported methods.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// validTokenStore returns a fixed token so the client reaches the server, letting
// us count HTTP requests and verify short-circuit behaviour.
type validTokenStore struct{ token string }

func (s validTokenStore) Get(_ string) (ports.Tokens, error) {
	return ports.Tokens{AccessToken: s.token}, nil
}
func (s validTokenStore) Put(_ string, _ ports.Tokens) error { return nil }
func (s validTokenStore) Delete(_ string) error              { return nil }

// TestRunPullShortCircuitsWhenUnauthorized verifies that runPull exits after the
// FIRST resource that returns ErrUnauthorized, making exactly ONE HTTP request
// instead of one per resource (3 base + 2 optional = 5 before the fix).
func TestRunPullShortCircuitsWhenUnauthorized(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		// Every request returns 401 — no refresher configured, so the client
		// maps this to ErrUnauthorized.
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// Client with a valid token so it actually reaches the server; no refresher
	// so a 401 response becomes ErrUnauthorized immediately.
	c := NewClient(srv.URL, validTokenStore{"bearer-tok"}, "test")

	ws := &testutil.FakeSyncWatermarkStore{}
	q := NewQueue(&testutil.FakeWriteQueue{})
	w := NewWorker(
		c,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		ws,
		q,
		"user1",
	)

	// Call runPull directly (white-box, package httpsync).
	w.runPull(context.Background())

	// After the fix: exactly 1 HTTP request (first resource — "projects" —
	// returns ErrUnauthorized and runPull returns early). Before the fix: 3
	// requests (projects + sessions + active_sessions, each logged as Warn).
	if got := requestCount.Load(); got != 1 {
		t.Errorf("HTTP requests: got %d, want 1 (runPull should short-circuit on first ErrUnauthorized)", got)
	}
}
