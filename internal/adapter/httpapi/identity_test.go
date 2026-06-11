package httpapi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

func TestIdentity_Me_Happy(t *testing.T) {
	api := newTestAPI(t)
	id := httpapi.NewIdentity(api.Client)

	user, err := id.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user.OIDCSub == "" {
		t.Error("expected OIDCSub to be set")
	}
	if user.Email == "" {
		t.Error("expected Email to be set")
	}
}

func TestIdentity_Me_Cached(t *testing.T) {
	api := newTestAPI(t)
	id := httpapi.NewIdentity(api.Client)
	ctx := context.Background()

	// First call fetches from server
	u1, err := id.Me(ctx)
	if err != nil {
		t.Fatalf("first Me: %v", err)
	}

	// Second call should return cached result
	u2, err := id.Me(ctx)
	if err != nil {
		t.Fatalf("second Me: %v", err)
	}
	if u1.OIDCSub != u2.OIDCSub {
		t.Errorf("cached sub differs: %q vs %q", u1.OIDCSub, u2.OIDCSub)
	}
}

func TestIdentity_Me_LoggedOut(t *testing.T) {
	api := newTestAPI(t)
	// Create a client without a valid token
	cli := httpapi.New(httpapi.Config{
		BaseURL: api.URL,
		Tokens:  &memTokens{ok: false},
		Slot:    "test",
	})
	id := httpapi.NewIdentity(cli)

	_, err := id.Me(context.Background())
	if !errors.Is(err, httpapi.ErrLoggedOut) {
		t.Errorf("expected ErrLoggedOut, got: %v", err)
	}
}

func TestIdentity_Me_NotConfigured(t *testing.T) {
	cli := httpapi.New(httpapi.Config{
		BaseURL: "",
		Tokens: &memTokens{
			ok:  true,
			tok: ports.Tokens{AccessToken: "dummy"},
		},
		Slot: "test",
	})
	id := httpapi.NewIdentity(cli)

	_, err := id.Me(context.Background())
	if !errors.Is(err, httpapi.ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got: %v", err)
	}
}
