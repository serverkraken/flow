package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestFakeUserStoreRoundTrip(t *testing.T) {
	s := NewFakeUserStore()
	if _, err := s.GetBySub(context.Background(), "x"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
	u, _ := domain.NewUser("id-1", "x", "u", "e", "n")
	if _, err := s.UpsertBySub(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBySub(context.Background(), "x")
	if err != nil || got.ID != "id-1" {
		t.Fatalf("round trip failed: %+v %v", got, err)
	}
}

func TestFakeIDGenMonotonic(t *testing.T) {
	g := &FakeIDGen{}
	if g.NewID() == g.NewID() {
		t.Fatal("ids should differ")
	}
}
