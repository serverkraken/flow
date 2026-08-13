package main

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

type commandTokenStore struct {
	locks  int
	saved  ports.Token
	clears int
}

func (s *commandTokenStore) WithLock(_ context.Context, fn func(ports.TokenStoreSession) error) error {
	s.locks++
	return fn(s)
}

func (s *commandTokenStore) Save(token ports.Token) error {
	s.saved = token
	return nil
}

func (s *commandTokenStore) Load() (ports.Token, bool, error) {
	return s.saved, s.saved.AccessToken != "", nil
}

func (s *commandTokenStore) Clear() error {
	s.saved = ports.Token{}
	s.clears++
	return nil
}

func TestLoginAndLogoutStoreOperationsUseLock(t *testing.T) {
	store := &commandTokenStore{}
	want := ports.Token{AccessToken: "access", RefreshToken: "refresh"}
	if err := saveStoredToken(context.Background(), store, want); err != nil {
		t.Fatal(err)
	}
	if store.locks != 1 || store.saved != want {
		t.Fatalf("after save: locks=%d saved=%+v", store.locks, store.saved)
	}
	if err := clearStoredToken(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if store.locks != 2 || store.clears != 1 || store.saved != (ports.Token{}) {
		t.Fatalf("after clear: locks=%d clears=%d saved=%+v", store.locks, store.clears, store.saved)
	}
}
