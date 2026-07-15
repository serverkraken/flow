package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestFakeEmbedder_DeterministicAndError(t *testing.T) {
	e := NewFakeEmbedder()
	a, err := e.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || len(a[0]) != e.Dim {
		t.Fatalf("shape wrong: %d vecs, dim %d", len(a), len(a[0]))
	}
	b, _ := e.Embed(context.Background(), []string{"hello"})
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("not deterministic at %d", i)
		}
	}
	e.Err = errTest
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("expected error when Err set")
	}
}

var errTest = errors.New("boom")

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

func TestFakeProjectBindingStore_UpsertReassignDelete(t *testing.T) {
	s := NewFakeProjectBindingStore()
	ctx := context.Background()
	_, _ = s.Upsert(ctx, domain.ProjectBinding{ID: "b1", OwnerID: "u", NodeID: "p1", Kind: domain.BindingRemote, RemoteSlug: "r"})
	_, _ = s.Upsert(ctx, domain.ProjectBinding{ID: "b2", OwnerID: "u", NodeID: "p2", Kind: domain.BindingRemote, RemoteSlug: "r"}) // reassign
	got, _ := s.List(ctx, "u")
	if len(got) != 1 || got[0].NodeID != "p2" {
		t.Fatalf("reassign: %+v", got)
	}
	if err := s.DeleteRemote(ctx, "u", "r"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(ctx, "u"); len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
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
		ID: id, OwnerID: owner, NodeID: proj, Type: domain.DocFree,
		Path: path, Title: id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFakeDocumentStore_Search(t *testing.T) {
	s := NewFakeDocumentStore()
	ctx := context.Background()
	mk := func(id, title, body string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a", "Kompendium", "about the compendium", "go")
	mk("b", "Other", "unrelated text")

	hits, err := s.Search(ctx, "u", "kompend", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("search kompend = %#v, want [a]", hits)
	}
	if !strings.Contains(hits[0].Snippet, domain.HighlightStart) {
		t.Fatalf("snippet missing highlight markers: %q", hits[0].Snippet)
	}
	none, _ := s.Search(ctx, "u", "kompend", nil, []string{"missing"})
	if len(none) != 0 {
		t.Fatalf("tag-filtered search = %d, want 0", len(none))
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

	all, _ := s.List(ctx, "u", nil)
	if len(all) != 3 {
		t.Fatalf("unfiltered = %d, want 3", len(all))
	}
	goDocs, _ := s.List(ctx, "u", nil, "go")
	if len(goDocs) != 2 {
		t.Fatalf("tag=go = %d, want 2", len(goDocs))
	}
	both, _ := s.List(ctx, "u", nil, "go", "tui")
	if len(both) != 1 || both[0].ID != "a" {
		t.Fatalf("tag=go,tui = %#v, want [a]", both)
	}
}

func TestFakeStore_ChunksAndSemantic(t *testing.T) {
	s := NewFakeDocumentStore()
	e := NewFakeEmbedder()
	ctx := context.Background()
	mk := func(id, title, body string, tags ...string) domain.Document {
		d, err := s.Create(ctx, domain.Document{ID: id, OwnerID: "u", Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	a := mk("a", "Alpha", "alpha body", "go")
	mk("b", "Beta", "beta body")

	stale, err := s.StaleDocuments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 2 {
		t.Fatalf("want 2 stale, got %d", len(stale))
	}

	texts := []string{"Alpha\n\nalpha body"}
	vecs, _ := e.Embed(ctx, texts)
	if err := s.ReplaceChunks(ctx, a.ID, a.OwnerID, s.SnapshotHash(a.ID), texts, vecs); err != nil {
		t.Fatal(err)
	}
	stale, _ = s.StaleDocuments(ctx, 10)
	if len(stale) != 1 || stale[0].Doc.ID != "b" {
		t.Fatalf("want only b stale, got %#v", stale)
	}

	hits, err := s.SemanticSearch(ctx, "u", vecs[0], nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" || hits[0].Snippet == "" {
		t.Fatalf("semantic want [a] with snippet, got %#v", hits)
	}
	none, _ := s.SemanticSearch(ctx, "u", vecs[0], nil, []string{"missing"}, 10)
	if len(none) != 0 {
		t.Fatalf("tag-filtered semantic want 0, got %d", len(none))
	}
}

func TestFakeDocumentStore_UpsertByPath_ConvergesType(t *testing.T) {
	s := NewFakeDocumentStore()
	ctx := context.Background()

	// Insert with DocMemory
	id1, _, err := s.UpsertByPath(ctx, "u", nil, domain.DocMemory, "active-context", "AC", "v1", false, false, "agent", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	got1, _ := s.Get(ctx, "u", id1)
	if got1.Type != domain.DocMemory {
		t.Fatalf("initial type: want memory, got %q", got1.Type)
	}

	// Re-upsert at same (owner, nil node, path) with DocActiveContext — must converge
	id2, _, err := s.UpsertByPath(ctx, "u", nil, domain.DocActiveContext, "active-context", "AC", "v2", false, false, "agent", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("type-converge upsert must reuse same id: %q vs %q", id1, id2)
	}
	got2, _ := s.Get(ctx, "u", id2)
	if got2.Type != domain.DocActiveContext {
		t.Fatalf("type after converge: want activecontext, got %q", got2.Type)
	}
}

// TestFakeFeedTokenStore_Revoke covers FakeFeedTokenStore.Revoke (at 0%).
func TestFakeFeedTokenStore_Revoke(t *testing.T) {
	s := NewFakeFeedTokenStore()
	ctx := context.Background()
	ft := domain.FeedToken{UserID: "u1", Token: "tok-abc"}
	if err := s.Create(ctx, ft); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Revoke with wrong userID should be a no-op (token stays).
	if err := s.Revoke(ctx, "wrong-user", "tok-abc"); err != nil {
		t.Fatalf("Revoke(wrong user): %v", err)
	}
	if uid, err := s.Resolve(ctx, "tok-abc"); err != nil || uid != "u1" {
		t.Errorf("after revoke with wrong user, token should still resolve: uid=%q err=%v", uid, err)
	}
	// Revoke with correct userID removes the token.
	if err := s.Revoke(ctx, "u1", "tok-abc"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := s.Resolve(ctx, "tok-abc"); err == nil {
		t.Error("after revoke, Resolve should return an error")
	}
}

// TestFakeProjectBindingStore_DeletePath covers FakeProjectBindingStore.DeletePath (at 0%).
func TestFakeProjectBindingStore_DeletePath(t *testing.T) {
	s := NewFakeProjectBindingStore()
	ctx := context.Background()
	b := domain.ProjectBinding{
		ID:        "b1",
		OwnerID:   "u1",
		NodeID:    "p1",
		Kind:      domain.BindingPath,
		MachineID: "mac-1",
		Path:      "/home/user/proj",
	}
	if _, err := s.Upsert(ctx, b); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// DeletePath removes the binding.
	if err := s.DeletePath(ctx, "u1", "mac-1", "/home/user/proj"); err != nil {
		t.Fatalf("DeletePath: %v", err)
	}
	all := s.All()
	for _, got := range all {
		if got.ID == "b1" {
			t.Error("binding should have been deleted by DeletePath")
		}
	}
}
