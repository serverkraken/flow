package oidcclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Tokens_Current_NotExpired_ReturnsCached(t *testing.T) {
	t.Parallel()
	store := keyringadapter.NewFake()
	_ = store.Put("slot", ports.Tokens{
		AccessToken: "still-valid",
		Expiry:      time.Now().Add(time.Hour),
	})

	tm := NewTokens(TokensConfig{Store: store, SlotName: "slot"})
	tok, err := tm.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if tok.AccessToken != "still-valid" {
		t.Fatalf("AccessToken = %q", tok.AccessToken)
	}
}

func TestUnit_Tokens_Current_NotLoggedIn_ReturnsErrTokenNotFound(t *testing.T) {
	t.Parallel()
	tm := NewTokens(TokensConfig{Store: keyringadapter.NewFake(), SlotName: "slot"})
	_, err := tm.Current(context.Background())
	if !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}

func TestUnit_Tokens_Current_Expired_TriggersRefreshAndPersists(t *testing.T) {
	t.Parallel()
	store := keyringadapter.NewFake()
	_ = store.Put("slot", ports.Tokens{
		AccessToken:  "old",
		RefreshToken: "r1",
		Expiry:       time.Now().Add(10 * time.Second), // within leeway
	})

	refreshCalled := false
	tm := NewTokens(TokensConfig{
		Store:    store,
		SlotName: "slot",
		Refresh: func(_ context.Context, r string) (ports.Tokens, error) {
			refreshCalled = true
			if r != "r1" {
				t.Errorf("refresh was given %q, want r1", r)
			}
			return ports.Tokens{
				AccessToken:  "new",
				RefreshToken: "r2",
				Expiry:       time.Now().Add(time.Hour),
			}, nil
		},
	})

	tok, err := tm.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !refreshCalled {
		t.Error("Refresh should have been called")
	}
	if tok.AccessToken != "new" {
		t.Errorf("AccessToken = %q, want new", tok.AccessToken)
	}
	// stored token is the refreshed one
	stored, _ := store.Get("slot")
	if stored.RefreshToken != "r2" {
		t.Errorf("stored RefreshToken = %q, want r2", stored.RefreshToken)
	}
}

func TestUnit_Tokens_Current_NoRefreshFunc_ReturnsExpiredAsIs(t *testing.T) {
	t.Parallel()
	store := keyringadapter.NewFake()
	_ = store.Put("slot", ports.Tokens{
		AccessToken:  "expired",
		RefreshToken: "r1",
		Expiry:       time.Now().Add(-time.Hour),
	})

	tm := NewTokens(TokensConfig{Store: store, SlotName: "slot"})
	tok, err := tm.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if tok.AccessToken != "expired" {
		t.Errorf("AccessToken = %q, want expired (best effort)", tok.AccessToken)
	}
}

func TestUnit_Tokens_SaveAndDelete(t *testing.T) {
	t.Parallel()
	store := keyringadapter.NewFake()
	tm := NewTokens(TokensConfig{Store: store, SlotName: "slot"})

	if err := tm.Save(ports.Tokens{AccessToken: "a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := store.Get("slot")
	if got.AccessToken != "a" {
		t.Errorf("AccessToken = %q", got.AccessToken)
	}

	if err := tm.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("slot"); !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("after Delete: err = %v", err)
	}
}
