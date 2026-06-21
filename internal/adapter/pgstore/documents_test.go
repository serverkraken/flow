package pgstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestDocumentStore_CRUDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// seed a user so the documents FK is satisfied
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-doc", "sub-doc", "docuser", "doc@x.de", "Doc User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	uid := "u-doc"

	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)

	d := domain.Document{
		ID:        "d1",
		OwnerID:   uid,
		Type:      domain.DocFree,
		Path:      "docs/arch",
		Title:     "Arch",
		Body:      "# Hi",
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Create
	got, err := st.Create(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "docs/arch" || got.Title != "Arch" {
		t.Fatalf("create roundtrip: %+v", got)
	}

	// Get
	g, err := st.Get(ctx, uid, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if g.Body != "# Hi" {
		t.Errorf("get body: %q", g.Body)
	}

	// List
	list, err := st.List(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len %d", len(list))
	}

	// Update
	d.Title = "Arch2"
	d.Body = "# Bye"
	d.UpdatedAt = now.Add(time.Minute)
	u2, err := st.Update(ctx, d)
	if err != nil || u2.Title != "Arch2" || u2.Body != "# Bye" {
		t.Fatalf("update: %+v err %v", u2, err)
	}

	// Duplicate path → ErrDocumentExists
	dup := domain.Document{
		ID:        "d2",
		OwnerID:   uid,
		Type:      domain.DocFree,
		Path:      "docs/arch",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := st.Create(ctx, dup); !errors.Is(err, ports.ErrDocumentExists) {
		t.Errorf("dup path: want ErrDocumentExists, got %v", err)
	}

	// Get unknown → ErrDocumentNotFound
	if _, err := st.Get(ctx, uid, "nope"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("get unknown: %v", err)
	}

	// Delete
	if err := st.Delete(ctx, uid, "d1"); err != nil {
		t.Fatal(err)
	}

	// Delete twice → ErrDocumentNotFound
	if err := st.Delete(ctx, uid, "d1"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestDocumentStore_ListTagFilter(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-tagf", "sub-tagf", "tagfuser", "tagf@x.de", "Tag Filter")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	owner := "u-tagf"

	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path string, tags ...string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, Type: domain.DocFree, Path: path, Tags: tags,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("tf-a", "a", "go", "tui")
	mk("tf-b", "b", "go")

	got, err := st.List(ctx, owner, nil, "go", "tui")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "a" {
		t.Fatalf("List(go,tui) = %#v, want [a]", got)
	}
}

func TestDocumentStore_SearchFuzzyAndTag(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-srch", "sub-srch", "srchuser", "srch@x.de", "Search User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	owner := "u-srch"

	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path, title, body string, tags ...string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, Type: domain.DocFree, Path: path,
			Title: title, Body: body, Tags: tags, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("srch-a", "a", "Kompendium", "notes about the compendium", "go")
	mk("srch-b", "b", "Anderes", "etwas ganz anderes")

	hits, err := st.Search(ctx, owner, "kompend", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "a" {
		t.Fatalf(`search "kompend" = %#v, want [a]`, hits)
	}
	// prefix/partial hit must also be highlighted (prefix tsquery union in ts_headline)
	if !strings.Contains(hits[0].Snippet, domain.HighlightStart) {
		t.Fatalf(`search "kompend": expected highlighted snippet (prefix hit), got %q`, hits[0].Snippet)
	}

	exact, err := st.Search(ctx, owner, "compendium", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) == 0 || !strings.Contains(exact[0].Snippet, domain.HighlightStart) {
		t.Fatalf("expected highlighted snippet, got %#v", exact)
	}

	// punctuation-only query: no error, no rows (edge case for ''::tsquery guard)
	punct, err := st.Search(ctx, owner, "!!!", nil, nil)
	if err != nil {
		t.Fatalf(`search "!!!": unexpected error: %v`, err)
	}
	if len(punct) != 0 {
		t.Fatalf(`search "!!!": expected 0 results, got %d`, len(punct))
	}

	none, err := st.Search(ctx, owner, "kompend", nil, []string{"missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("tag-filtered search = %d, want 0", len(none))
	}
}

func TestDocumentStore_Links(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-lnk", "sub-lnk", "lnk", "lnk@x.de", "Lnk")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path string) {
		if _, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: "u-lnk", Type: domain.DocFree, Path: path,
			Title: id, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("s1", "alpha")
	mk("s2", "beta")

	if err := st.ReplaceLinks(ctx, "s1", "u-lnk", []string{"beta", "gamma"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceLinks(ctx, "s2", "u-lnk", []string{"beta"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Backlinks(ctx, "u-lnk", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("backlinks(beta) = %d docs, want 2", len(got))
	}

	if err := st.ReplaceLinks(ctx, "s1", "u-lnk", []string{"gamma"}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Backlinks(ctx, "u-lnk", "beta")
	if len(got) != 1 || got[0].ID != "s2" {
		t.Fatalf("after replace, backlinks(beta) = %v, want only s2", got)
	}

	if err := st.Delete(ctx, "u-lnk", "s2"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Backlinks(ctx, "u-lnk", "beta")
	if len(got) != 0 {
		t.Fatalf("after delete s2, backlinks(beta) = %v, want none", got)
	}
}

func TestDocumentStore_SemanticSearch(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("sem-owner", "sub-sem", "semuser", "sem@x.de", "Sem User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	owner := "sem-owner"

	s := pgstore.NewDocumentStore(pool)
	now := time.Now().UTC().Truncate(time.Second)

	vec := func(v float32) []float32 {
		out := make([]float32, 768)
		for i := range out {
			out[i] = v
		}
		return out
	}

	mkDoc := func(id, title, body string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: owner, Type: domain.DocFree, Path: id, Title: title, Body: body, Tags: tags, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	mkDoc("near", "Near", "near doc", "go")
	mkDoc("far", "Far", "far doc")

	if err := s.ReplaceChunks(ctx, "near", owner, []string{"near chunk"}, [][]float32{vec(0.9)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "far", owner, []string{"far chunk"}, [][]float32{vec(-0.9)}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.SemanticSearch(ctx, owner, vec(1.0), nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "near" {
		t.Fatalf("want near first, got %#v", hits)
	}
	if hits[0].Snippet != "near chunk" {
		t.Fatalf("snippet = %q, want near chunk", hits[0].Snippet)
	}
	tagged, _ := s.SemanticSearch(ctx, owner, vec(1.0), nil, []string{"go"}, 10)
	if len(tagged) != 1 || tagged[0].Path != "near" {
		t.Fatalf("tag-filtered semantic = %#v, want [near]", tagged)
	}
	mkDoc("fresh", "Fresh", "fresh")
	stale, _ := s.StaleDocuments(ctx, 100)
	foundFresh := false
	for _, d := range stale {
		if d.Path == "fresh" {
			foundFresh = true
		}
		if d.Path == "near" {
			t.Fatalf("near should not be stale after ReplaceChunks")
		}
	}
	if !foundFresh {
		t.Fatal("fresh doc should be stale")
	}
}
