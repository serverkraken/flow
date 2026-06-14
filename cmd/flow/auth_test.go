package main

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

// fakeSource hands out a fixed token.
type fakeSource struct{ tok *oauth2.Token }

func (f fakeSource) Token() (*oauth2.Token, error) { return f.tok, nil }

// memStore is an in-memory TokenStore.
type memStore struct {
	saved ports.Token
	calls int
}

func (m *memStore) Save(t ports.Token) error         { m.saved = t; m.calls++; return nil }
func (m *memStore) Load() (ports.Token, bool, error) { return m.saved, m.calls > 0, nil }
func (m *memStore) Clear() error                     { m.saved = ports.Token{}; return nil }

func TestPersistingSourceSavesOnChange(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "r", Expiry: time.Unix(10, 0)}},
		store: store,
		last:  ports.Token{AccessToken: "old"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.saved.AccessToken != "new" {
		t.Fatalf("expected save of new token, got calls=%d saved=%+v", store.calls, store.saved)
	}
	// Second call with an unchanged token must not re-save.
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("expected no re-save, calls=%d", store.calls)
	}
}

func TestPersistingSourcePreservesRefreshWhenEmpty(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "a2", RefreshToken: ""}},
		store: store,
		last:  ports.Token{AccessToken: "a1", RefreshToken: "keep"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.saved.RefreshToken != "keep" {
		t.Fatalf("refresh token not preserved: %q", store.saved.RefreshToken)
	}
}
