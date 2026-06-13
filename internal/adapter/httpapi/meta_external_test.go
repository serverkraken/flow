package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

func TestCheckMeta_SetsOutdatedWhenClientTooOld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/meta" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"server_version":"2.0.0","min_client_version":"2.0.0"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	store := &memTokens{ok: true, tok: ports.Tokens{AccessToken: "tok"}}
	cli := httpapi.New(httpapi.Config{BaseURL: srv.URL, Tokens: store, Slot: "s", Version: "1.0.0"})
	if err := cli.CheckMeta(context.Background()); err != nil {
		t.Fatalf("CheckMeta: %v", err)
	}
	if snap := cli.StatusOf().Snapshot(); snap.State != httpapi.StateOutdated {
		t.Errorf("state = %d, want StateOutdated", snap.State)
	}
}

func TestCheckMeta_NoOutdatedWhenVersionCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/meta" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"server_version":"2.0.0","min_client_version":"1.0.0"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	store := &memTokens{ok: true, tok: ports.Tokens{AccessToken: "tok"}}
	cli := httpapi.New(httpapi.Config{BaseURL: srv.URL, Tokens: store, Slot: "s", Version: "2.0.0"})
	if err := cli.CheckMeta(context.Background()); err != nil {
		t.Fatalf("CheckMeta: %v", err)
	}
	if snap := cli.StatusOf().Snapshot(); snap.State == httpapi.StateOutdated {
		t.Errorf("state = StateOutdated, want not outdated for current version")
	}
}

func TestCheckMeta_ErrNotConfigured_WhenNoBaseURL(t *testing.T) {
	store := &memTokens{ok: true, tok: ports.Tokens{AccessToken: "tok"}}
	cli := httpapi.New(httpapi.Config{BaseURL: "", Tokens: store, Slot: "s", Version: "1.0.0"})
	err := cli.CheckMeta(context.Background())
	if err == nil {
		t.Fatal("expected error for empty BaseURL, got nil")
	}
}
