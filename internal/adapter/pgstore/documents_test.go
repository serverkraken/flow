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
	"github.com/serverkraken/flow/internal/testutil"
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

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
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

func TestDocumentStore_MoveMetadataIsOwnerScopedAndCollisionSafe(t *testing.T) {
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
	for _, u := range []domain.User{
		{ID: "move-u1", OIDCSub: "move-sub-1", Username: "move1", Email: "move1@example.test"},
		{ID: "move-u2", OIDCSub: "move-sub-2", Username: "move2", Email: "move2@example.test"},
	} {
		if _, err := users.UpsertBySub(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	nodes := pgstore.NewNodeStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	for _, n := range []domain.Node{
		{ID: "move-n1", OwnerID: "move-u1", Kind: domain.KindEngagement, Name: "Own", Slug: "own", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
		{ID: "move-n2", OwnerID: "move-u2", Kind: domain.KindEngagement, Name: "Foreign", Slug: "foreign", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	ownNode := "move-n1"
	for _, d := range []domain.Document{
		{ID: "move-d1", OwnerID: "move-u1", Type: domain.DocFree, Path: "notes/source", CreatedAt: now, UpdatedAt: now},
		{ID: "move-d2", OwnerID: "move-u1", Type: domain.DocProject, NodeID: &ownNode, Path: "readme", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	collision := domain.Document{ID: "move-d1", OwnerID: "move-u1", Type: domain.DocProject, NodeID: &ownNode, Path: "readme", UpdatedAt: now.Add(time.Minute)}
	if _, err := docs.Move(ctx, collision); !errors.Is(err, ports.ErrDocumentExists) {
		t.Fatalf("collision: want ErrDocumentExists, got %v", err)
	}
	foreignNode := "move-n2"
	foreign := collision
	foreign.NodeID, foreign.Path = &foreignNode, "foreign"
	if _, err := docs.Move(ctx, foreign); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("foreign node: want ErrNodeNotFound, got %v", err)
	}

	valid := collision
	valid.Path = "overview"
	got, err := docs.Move(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != domain.DocProject || got.NodeID == nil || *got.NodeID != ownNode || got.Path != "overview" {
		t.Fatalf("moved = %+v", got)
	}
}

// TestDocumentStore_ProvenanceRoundTrip covers Task 3 (Migration 0028): the
// updated_by_kind/updated_by_ref stamp survives Get AND Search (Codex #8 —
// Search scans via prefixedDocCols/scanSearchHit, a separate column list +
// scanner from docCols/scanDocument, so it needs its own coverage). A doc
// inserted the pre-L3 way (no provenance columns) must read back as empty
// strings without a scan error.
func TestDocumentStore_ProvenanceRoundTrip(t *testing.T) {
	t.Parallel()
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
	u, _ := domain.NewUser("u-prov", "sub-prov", "provuser", "prov@x.de", "Prov User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	owner := "u-prov"

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)

	// Doc written with a known actor (agent).
	stamped := domain.Document{
		ID: "prov-1", OwnerID: owner, Type: domain.DocFree, Path: "prov/stamped",
		Title: "Prov Stamped", Body: "kompendium body", CreatedAt: now, UpdatedAt: now,
		UpdatedByKind: "agent", UpdatedByRef: "claude-code",
	}
	if _, err := st.Create(ctx, stamped); err != nil {
		t.Fatal(err)
	}

	// Doc inserted the pre-L3 way, bypassing provenance columns entirely
	// (simulates a row from before migration 0028; NULL, not empty string).
	if _, err := pool.Exec(ctx, `INSERT INTO documents (id, owner_id, type, path, title, body, extra, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,'{}',$7,$7)`,
		"prov-2", owner, string(domain.DocFree), "prov/legacy", "Prov Legacy", "legacy body", now); err != nil {
		t.Fatal(err)
	}

	// Get: stamped doc carries the actor.
	got, err := st.Get(ctx, owner, "prov-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedByKind != "agent" || got.UpdatedByRef != "claude-code" {
		t.Fatalf("Get UpdatedByKind/Ref = %q/%q, want agent/claude-code", got.UpdatedByKind, got.UpdatedByRef)
	}

	// Get: legacy (NULL) doc reads back as empty strings, no scan error.
	gotLegacy, err := st.Get(ctx, owner, "prov-2")
	if err != nil {
		t.Fatal(err)
	}
	if gotLegacy.UpdatedByKind != "" || gotLegacy.UpdatedByRef != "" {
		t.Fatalf("Get legacy UpdatedByKind/Ref = %q/%q, want empty/empty", gotLegacy.UpdatedByKind, gotLegacy.UpdatedByRef)
	}

	// Search (prefixedDocCols + scanSearchHit path, Codex #8): both docs
	// surface provenance correctly, including the NULL legacy row.
	hits, err := st.Search(ctx, owner, "kompendium", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "prov-1" {
		t.Fatalf(`search "kompendium" = %#v, want [prov-1]`, hits)
	}
	if hits[0].UpdatedByKind != "agent" || hits[0].UpdatedByRef != "claude-code" {
		t.Fatalf("Search UpdatedByKind/Ref = %q/%q, want agent/claude-code", hits[0].UpdatedByKind, hits[0].UpdatedByRef)
	}

	legacyHits, err := st.Search(ctx, owner, "legacy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyHits) != 1 || legacyHits[0].UpdatedByKind != "" || legacyHits[0].UpdatedByRef != "" {
		t.Fatalf(`search "legacy" provenance = %#v, want empty/empty`, legacyHits)
	}

	// SemanticSearch (prefixedDocCols + scanSemanticHit path, Codex #8): an
	// embedder is not wired in this suite, but chunks can be seeded directly.
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = 0.5
	}
	if err := st.ReplaceChunks(ctx, "prov-1", owner, snapshotHash(t, st, "prov-1"), []string{"chunk"}, [][]float32{vec}); err != nil {
		t.Fatal(err)
	}
	semHits, err := st.SemanticSearch(ctx, owner, vec, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(semHits) != 1 || semHits[0].UpdatedByKind != "agent" || semHits[0].UpdatedByRef != "claude-code" {
		t.Fatalf("SemanticSearch UpdatedByKind/Ref = %#v, want agent/claude-code", semHits)
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

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path string, tags ...string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, Type: domain.DocFree, Path: path,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(tags) > 0 {
			if _, err := ts.SetTags(ctx, owner, domain.TaggableDocument, id, tags); err != nil {
				t.Fatal(err)
			}
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
	// Tags must be hydrated from taggings (not the legacy column).
	if len(got[0].Tags) != 2 {
		t.Fatalf("List: hydrated tags want 2, got %v", got[0].Tags)
	}
}

func TestDocumentStore_ListPage(t *testing.T) {
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
	u, _ := domain.NewUser("u-page", "sub-page", "pageuser", "page@x.de", "Page User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	other, _ := domain.NewUser("other", "sub-page-other", "otheruser", "other@x.de", "Other User")
	if _, err := users.UpsertBySub(ctx, other); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string, updated time.Time) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: "u-page", Type: domain.DocFree, Path: id,
			CreatedAt: now, UpdatedAt: updated,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ts.SetTags(ctx, "u-page", domain.TaggableDocument, id, []string{"go"}); err != nil {
			t.Fatal(err)
		}
	}
	mk("page-old", now)
	mk("page-mid", now.Add(time.Minute))
	mk("page-new", now.Add(2*time.Minute))
	_, err = st.Create(ctx, domain.Document{
		ID: "page-foreign", OwnerID: "other", Type: domain.DocFree, Path: "foreign",
		CreatedAt: now, UpdatedAt: now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, total, err := st.ListPage(ctx, "u-page", nil, 2, 1, "go")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total=%d want 3", total)
	}
	if len(got) != 2 || got[0].ID != "page-mid" || got[1].ID != "page-old" {
		t.Fatalf("page = %#v, want [page-mid page-old]", got)
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

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, path, title, body string, tags ...string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, Type: domain.DocFree, Path: path,
			Title: title, Body: body, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(tags) > 0 {
			if _, err := ts.SetTags(ctx, owner, domain.TaggableDocument, id, tags); err != nil {
				t.Fatal(err)
			}
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

// TestDocumentStore_SearchIncludesNieContextMode guards the L5.5 requirement
// that context_mode="nie" only hides a document from agent-context compose —
// it stays fully visible in Wissen/search. Search must NOT filter by
// context_mode (Codex-Fund #4).
func TestDocumentStore_SearchIncludesNieContextMode(t *testing.T) {
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
	u, _ := domain.NewUser("u-nie", "sub-nie", "nieuser", "nie@x.de", "Nie User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	owner := "u-nie"

	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	d, err := st.Create(ctx, domain.Document{
		ID: "nie-doc", OwnerID: owner, Type: domain.DocFree, Path: "nie-path",
		Title: "Versteckt", Body: "geheimer Suchtext kompendium", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetContextMode(ctx, owner, d.ID, domain.ContextModeNie); err != nil {
		t.Fatal(err)
	}

	hits, err := st.Search(ctx, owner, "kompendium", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "nie-doc" {
		t.Fatalf("nie-mode doc must still be found by Search, got %#v", hits)
	}
	if hits[0].ContextMode != domain.ContextModeNie {
		t.Fatalf("search hit ContextMode = %q, want nie", hits[0].ContextMode)
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
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
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

	s := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)

	vec := func(v float32) []float32 {
		out := make([]float32, 768)
		for i := range out {
			out[i] = v
		}
		return out
	}

	mkDoc := func(id, title, body string, tags ...string) {
		if _, err := s.Create(ctx, domain.Document{ID: id, OwnerID: owner, Type: domain.DocFree, Path: id, Title: title, Body: body, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if len(tags) > 0 {
			if _, err := ts.SetTags(ctx, owner, domain.TaggableDocument, id, tags); err != nil {
				t.Fatal(err)
			}
		}
	}
	mkDoc("near", "Near", "near doc", "go")
	mkDoc("far", "Far", "far doc")

	if err := s.ReplaceChunks(ctx, "near", owner, snapshotHash(t, s, "near"), []string{"near chunk"}, [][]float32{vec(0.9)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "far", owner, snapshotHash(t, s, "far"), []string{"far chunk"}, [][]float32{vec(-0.9)}); err != nil {
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
	for _, sd := range stale {
		if sd.Doc.Path == "fresh" {
			foundFresh = true
		}
		if sd.Doc.Path == "near" {
			t.Fatalf("near should not be stale after ReplaceChunks")
		}
	}
	if !foundFresh {
		t.Fatal("fresh doc should be stale")
	}
}

func TestDocumentStore_HydratesTagsFromJunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	us := pgstore.NewUserStore(pool)
	seedUser(t, us, "u1")
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	tags := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})

	d, err := docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "T", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tags.SetTags(ctx, "u1", domain.TaggableDocument, d.ID, []string{"go", "tui"}); err != nil {
		t.Fatal(err)
	}

	got, err := docs.Get(ctx, "u1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("Get hydrate want 2 tags, got %+v", got.Tags)
	}

	// AND filter via junction.
	list, err := docs.List(ctx, "u1", nil, "go", "tui")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "d1" {
		t.Fatalf("junction AND filter want [d1], got %+v", list)
	}
}

func TestDocumentStore_NoTagsHydratesEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil { // applies through 0020
		t.Fatal(err)
	}
	us := pgstore.NewUserStore(pool)
	seedUser(t, us, "u1")
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	d, err := docs.Create(ctx, domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "T", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tags) != 0 {
		t.Fatalf("want no tags, got %+v", d.Tags)
	}
}

func newDocStore(t *testing.T) (*pgstore.DocumentStore, *pgstore.UserStore, *pgstore.NodeStore, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{}), pgstore.NewUserStore(pool), pgstore.NewNodeStore(pool), func() { pool.Close() }
}

func seedNode(t *testing.T, ns *pgstore.NodeStore, ownerID, nodeID string) {
	t.Helper()
	n, err := domain.NewNode(nodeID, ownerID, "Node "+nodeID, nodeID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindEngagement // root nodes must be engagement
	if _, err := ns.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentStore_SetPinned(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	d, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "p1", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Pinned {
		t.Fatalf("new doc should default pinned=false")
	}
	if err := ds.SetPinned(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := ds.Get(ctx, "u1", "d1")
	if !got.Pinned {
		t.Fatalf("SetPinned(true) not reflected: %+v", got)
	}
}

func TestDocumentStore_SetPriority(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	d, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m/p", Title: "T", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Priority != 0 {
		t.Fatalf("new doc should default priority=0, got %d", d.Priority)
	}
	if err := ds.SetPriority(ctx, "u1", d.ID, 7); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	got, _ := ds.Get(ctx, "u1", d.ID)
	if got.Priority != 7 {
		t.Fatalf("Priority = %d, want 7", got.Priority)
	}

	// Owner-Scope: fremder Owner darf nicht schreiben.
	if err := ds.SetPriority(ctx, "u2", d.ID, 3); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("cross-owner SetPriority err = %v, want ErrDocumentNotFound", err)
	}
	got, _ = ds.Get(ctx, "u1", d.ID)
	if got.Priority != 7 {
		t.Fatalf("cross-owner SetPriority mutated priority: got %d, want 7", got.Priority)
	}
}

func TestDocumentStore_SetContextMode(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	d, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m/p", Title: "T", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ContextMode != domain.ContextModeAuto {
		t.Fatalf("new doc ContextMode = %q, want auto (DB default via Create OrAuto)", d.ContextMode)
	}
	if err := ds.SetContextMode(ctx, "u1", d.ID, domain.ContextModeImmer); err != nil {
		t.Fatalf("SetContextMode: %v", err)
	}
	got, _ := ds.Get(ctx, "u1", d.ID)
	if got.ContextMode != domain.ContextModeImmer {
		t.Fatalf("ContextMode = %q, want immer", got.ContextMode)
	}

	// Owner-Scope: fremder Owner darf nicht schreiben.
	if err := ds.SetContextMode(ctx, "u2", d.ID, domain.ContextModeNie); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("cross-owner SetContextMode err = %v, want ErrDocumentNotFound", err)
	}
	got, _ = ds.Get(ctx, "u1", d.ID)
	if got.ContextMode != domain.ContextModeImmer {
		t.Fatalf("cross-owner SetContextMode mutated context_mode: got %q, want immer", got.ContextMode)
	}
}

func TestDocumentStore_ArchivedRoundTrip(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	d := domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m1",
		Title: "M1", Body: "b", Archived: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := ds.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := ds.Get(ctx, "u1", "d1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Archived {
		t.Fatalf("archived not persisted: %+v", got)
	}

	// non-nil ArchivedAt round-trip
	someTime := time.Now().Truncate(time.Second)
	d2 := domain.Document{
		ID: "d2", OwnerID: "u1", Type: domain.DocMemory, Path: "m2",
		Title: "M2", Body: "b2", Archived: true, ArchivedAt: &someTime,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := ds.Create(ctx, d2); err != nil {
		t.Fatalf("create d2: %v", err)
	}
	got2, err := ds.Get(ctx, "u1", "d2")
	if err != nil {
		t.Fatalf("get d2: %v", err)
	}
	if got2.ArchivedAt == nil {
		t.Fatalf("ArchivedAt not persisted: %+v", got2)
	}
	if !got2.ArchivedAt.Truncate(time.Second).Equal(someTime) {
		t.Fatalf("ArchivedAt mismatch: want %v, got %v", someTime, got2.ArchivedAt)
	}
}

func TestDocumentStore_SetArchived(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	if _, err := ds.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocMemory, Path: "m1", Title: "M1",
		Pinned: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetArchived(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	got, _ := ds.Get(ctx, "u1", "d1")
	if !got.Archived || got.ArchivedAt == nil {
		t.Fatalf("archived/archived_at not set: %+v", got)
	}
	if got.Pinned {
		t.Fatalf("archiving must clear pinned: %+v", got)
	}
	if err := ds.SetArchived(ctx, "u1", "d1", false); err != nil {
		t.Fatal(err)
	}
	got, _ = ds.Get(ctx, "u1", "d1")
	if got.Archived || got.ArchivedAt != nil {
		t.Fatalf("un-archive must clear: %+v", got)
	}
}

func TestDocumentStore_UpsertByPath_InsertThenUpdate(t *testing.T) {
	t.Parallel()
	ds, us, ns, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	nid := "n1"
	seedNode(t, ns, "u1", nid)

	id1, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v1 body", false, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	id2, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v2 body", false, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("upsert must reuse the same row: %q vs %q", id1, id2)
	}
	got, _ := ds.Get(ctx, "u1", id1)
	if got.Body != "v2 body" {
		t.Fatalf("body not updated: %q", got.Body)
	}
}

func TestDocumentStore_UpsertByPath_ConvergesType(t *testing.T) {
	t.Parallel()
	ds, us, ns, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	nid := "n1"
	seedNode(t, ns, "u1", nid)

	// Insert as memory type
	id1, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v1 body", false, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got1, _ := ds.Get(ctx, "u1", id1)
	if got1.Type != domain.DocMemory {
		t.Fatalf("initial type: want memory, got %q", got1.Type)
	}

	// Re-upsert at same path with activecontext type — must converge
	id2, _, err := ds.UpsertByPath(ctx, "u1", &nid, domain.DocActiveContext, "active-context", "AC", "v2 body", false, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("type-converge upsert must reuse same row: %q vs %q", id1, id2)
	}
	got2, _ := ds.Get(ctx, "u1", id2)
	if got2.Type != domain.DocActiveContext {
		t.Fatalf("type after converge: want activecontext, got %q", got2.Type)
	}
}

func TestDocumentStore_UpsertByPath_GlobalNodeNull(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	id1, _, err := ds.UpsertByPath(ctx, "u1", nil, domain.DocMemory, "active-context", "G", "g1", false, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	id2, _, _ := ds.UpsertByPath(ctx, "u1", nil, domain.DocMemory, "active-context", "G", "g2", false, false, "", "")
	if id1 != id2 {
		t.Fatalf("global (node_id NULL) upsert must hit the coalesce('') index, got %q vs %q", id1, id2)
	}
}

func TestDocumentStore_UpsertByPath_PreservesPin(t *testing.T) {
	t.Parallel()
	ds, us, ns, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	nid := "n1"
	seedNode(t, ns, "u1", nid)
	id, _, _ := ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v1", true, false, "", "")
	_, _, _ = ds.UpsertByPath(ctx, "u1", &nid, domain.DocMemory, "active-context", "AC", "v2", false, false, "", "") // flush, pinned arg false
	got, _ := ds.Get(ctx, "u1", id)
	if !got.Pinned {
		t.Fatalf("upsert-on-conflict must PRESERVE the existing pin")
	}
}

func TestDocumentStore_ListForContext(t *testing.T) {
	t.Parallel()
	ds, us, ns, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	leaf, eng := "leafN", "engN"
	seedNode(t, ns, "u1", leaf)
	seedNode(t, ns, "u1", eng)

	mk := func(id, typ string, node *string) {
		if _, err := ds.Create(ctx, domain.Document{ID: id, OwnerID: "u1", NodeID: node, Type: domain.DocumentType(typ), Path: id, Title: id, Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	mk("i-leaf", "instruction", &leaf)
	mk("m-leaf", "memory", &leaf)
	mk("m-eng", "memory", &eng)
	mk("i-glob", "instruction", nil)
	mk("daily-leaf", "daily", &leaf) // must be excluded by type filter

	got, err := ds.ListForContext(ctx, "u1", []string{leaf, eng}, true,
		[]domain.DocumentType{domain.DocInstruction, domain.DocMemory})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	for _, want := range []string{"i-leaf", "m-leaf", "m-eng", "i-glob"} {
		if !ids[want] {
			t.Errorf("ListForContext missing %s; got %v", want, ids)
		}
	}
	if ids["daily-leaf"] {
		t.Errorf("type filter leaked a daily doc")
	}
}

func TestDocumentStore_ArchivedExcludedFromReads(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	mk := func(id string, archived bool) {
		if _, err := ds.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocMemory, Path: id, Title: "needle " + id,
			Body: "needle body", Archived: archived, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("live", false)
	mk("arch", true)

	list, _ := ds.List(ctx, "u1", nil)
	if containsID(list, "arch") || !containsID(list, "live") {
		t.Fatalf("List must exclude archived; got IDs: %v", ids(list))
	}
	hits, _ := ds.Search(ctx, "u1", "needle", nil, nil)
	for _, h := range hits {
		if h.ID == "arch" {
			t.Fatalf("Search must exclude archived")
		}
	}
	ctxDocs, _ := ds.ListForContext(ctx, "u1", nil, true, []domain.DocumentType{domain.DocMemory})
	if containsID(ctxDocs, "arch") {
		t.Fatalf("ListForContext must exclude archived")
	}
}

func TestDocumentStore_ListArchived(t *testing.T) {
	t.Parallel()
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	_, _ = ds.Create(ctx, domain.Document{ID: "live", OwnerID: "u1", Type: domain.DocMemory, Path: "live", Title: "L", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_, _ = ds.Create(ctx, domain.Document{ID: "arch", OwnerID: "u1", Type: domain.DocMemory, Path: "arch", Title: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = ds.SetArchived(ctx, "u1", "arch", true)
	got, err := ds.ListArchived(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "arch" {
		t.Fatalf("want [arch], got %v", ids(got))
	}
}

func containsID(docs []domain.Document, id string) bool {
	for _, d := range docs {
		if d.ID == id {
			return true
		}
	}
	return false
}
