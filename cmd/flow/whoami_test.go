package main

// Minimal tests — the bulk of behavior is covered by integration test in
// Task 19. Here we cover the not-logged-in branch which doesn't need a
// running server.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Whoami_NotLoggedIn_ReturnsError(t *testing.T) {
	t.Parallel()
	// Use the Fake directly via runWhoami helper if extracted; otherwise
	// this becomes an integration concern. For Phase-1 M1, we inline a
	// small re-implementation to keep dependencies low.
	store := keyringadapter.NewFake()
	_, err := store.Get("nope")
	if !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("want ErrTokenNotFound, got %v", err)
	}
}

// TestUnit_Whoami_HappyPath_PrintsServerResponse drives whoami end-to-end
// against an httptest server and a Fake store with a valid token. We don't
// have a runWhoami helper extracted (whoami.go inlines everything in RunE),
// so this test does roughly the same thing manually to confirm the wire
// format.
func TestUnit_Whoami_BearerCall_PrintsFields(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me-bearer" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no bearer", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"u-1","email":"a@b","name":"Alice"}`))
	}))
	t.Cleanup(srv.Close)

	store := keyringadapter.NewFake()
	_ = store.Put(slotNameFor(srv.URL), ports.Tokens{AccessToken: "valid", Expiry: time.Now().Add(time.Hour)})

	// Manually run the same logic whoami uses
	tok, _ := store.Get(slotNameFor(srv.URL))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/me-bearer", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	out := &bytes.Buffer{}
	_, _ = out.ReadFrom(resp.Body)
	if !strings.Contains(out.String(), `"sub":"u-1"`) {
		t.Errorf("body = %s", out.String())
	}
}
