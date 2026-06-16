package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

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
	a, b := g.NewID(), g.NewID()
	if a == b {
		t.Fatal("ids should differ")
	}
}

func TestFakeDocumentStore_Links(t *testing.T) {
	ctx := context.Background()
	s := NewFakeDocumentStore()
	mustCreate(t, s, "src1", "owner", "a", nil)
	mustCreate(t, s, "src2", "owner", "b", nil)

	if err := s.ReplaceLinks(ctx, "src1", "owner", []string{"b", "c"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceLinks(ctx, "src2", "owner", []string{"b"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Backlinks(ctx, "owner", "b")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	if !ids["src1"] || !ids["src2"] || len(got) != 2 {
		t.Fatalf("backlinks of b = %v, want src1+src2", ids)
	}

	if err := s.ReplaceLinks(ctx, "src1", "owner", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Backlinks(ctx, "owner", "b")
	if len(got) != 1 || got[0].ID != "src2" {
		t.Fatalf("after clear, backlinks of b = %v, want only src2", got)
	}

	other, _ := s.Backlinks(ctx, "stranger", "b")
	if len(other) != 0 {
		t.Fatalf("expected no cross-owner backlinks, got %v", other)
	}
}

func mustCreate(t *testing.T, s *FakeDocumentStore, id, owner, path string, proj *string) {
	t.Helper()
	_, err := s.Create(context.Background(), domain.Document{
		ID: id, OwnerID: owner, ProjectID: proj, Type: domain.DocFree,
		Path: path, Title: id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFakeDocumentStore_ListTagFilter(t *testing.T) {
	s := NewFakeDocumentStore()
	ctx := context.Background()
	mk := func(id string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "go", "tui")
	mk("b", "go")
	mk("c", "web")

	all, _ := s.List(ctx, "u")
	if len(all) != 3 {
		t.Fatalf("unfiltered = %d, want 3", len(all))
	}
	goDocs, _ := s.List(ctx, "u", "go")
	if len(goDocs) != 2 {
		t.Fatalf("tag=go = %d, want 2", len(goDocs))
	}
	both, _ := s.List(ctx, "u", "go", "tui")
	if len(both) != 1 || both[0].ID != "a" {
		t.Fatalf("tag=go,tui = %#v, want [a]", both)
	}
}
