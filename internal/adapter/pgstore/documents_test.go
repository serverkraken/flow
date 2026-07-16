package pgstore_test

import (
	"context"
	"errors"
	"strings"
	"sync"
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

func TestDocumentStore_ListLibraryPageFacetsAreOwnerScopedAndFilterConsistent(t *testing.T) {
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
	for _, user := range []domain.User{
		func() domain.User {
			u, _ := domain.NewUser("u-library", "sub-library", "library", "library@x.de", "Library")
			return u
		}(),
		func() domain.User {
			u, _ := domain.NewUser("u-library-other", "sub-library-other", "library-other", "other@x.de", "Other")
			return u
		}(),
	} {
		if _, err := users.UpsertBySub(ctx, user); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	nodes := pgstore.NewNodeStore(pool)
	if _, err := nodes.Create(ctx, domain.Node{
		ID: "lib-root", OwnerID: "u-library", Kind: domain.KindEngagement, Name: "Library", Slug: "library",
		Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	rootID := "lib-root"
	for _, id := range []string{"lib-n1", "lib-n2", "lib-outside"} {
		if _, err := nodes.Create(ctx, domain.Node{
			ID: id, OwnerID: "u-library", ParentID: &rootID, Kind: domain.KindRepo, Name: id, Slug: id,
			Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	store := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	tags := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	create := func(id, owner, nodeID string, typ domain.DocumentType, updated time.Time, tag string) {
		nid := nodeID
		if _, err := store.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, NodeID: &nid, Type: typ, Path: id, Title: id, Body: "alpha library",
			CreatedAt: now, UpdatedAt: updated,
		}); err != nil {
			t.Fatal(err)
		}
		if tag != "" {
			if _, err := tags.SetTags(ctx, owner, domain.TaggableDocument, id, []string{tag}); err != nil {
				t.Fatal(err)
			}
		}
	}
	create("lib-n1-active", "u-library", "lib-n1", domain.DocPlan, now, "ops")
	create("lib-n2-active", "u-library", "lib-n2", domain.DocPlan, now.Add(time.Minute), "ops")
	create("lib-n2-archived", "u-library", "lib-n2", domain.DocPlan, now.Add(2*time.Minute), "ops")
	create("lib-wrong-type", "u-library", "lib-n2", domain.DocMemory, now.Add(3*time.Minute), "ops")
	create("lib-wrong-tag", "u-library", "lib-n2", domain.DocPlan, now.Add(4*time.Minute), "design")
	create("lib-outside", "u-library", "lib-outside", domain.DocPlan, now.Add(5*time.Minute), "ops")
	// Deliberately attach another owner's document to an existing node ID. The
	// library query must still exclude it solely through owner_id scoping.
	create("lib-foreign", "u-library-other", "lib-n2", domain.DocPlan, now.Add(6*time.Minute), "ops")
	if err := store.SetArchived(ctx, "u-library", "lib-n2-archived", true); err != nil {
		t.Fatal(err)
	}

	page, err := store.ListLibraryPage(ctx, "u-library", ports.DocumentLibraryQuery{
		NodeIDs:       []string{"lib-n1", "lib-n2"},
		FilterNodeIDs: true,
		Types:         []domain.DocumentType{domain.DocPlan},
		Tags:          []string{"ops"},
		Status:        ports.DocumentLibraryAll,
		Limit:         2,
		Offset:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.ActiveTotal != 2 || page.ArchivedTotal != 1 {
		t.Fatalf("library facets = total:%d active:%d archived:%d, want 3/2/1", page.Total, page.ActiveTotal, page.ArchivedTotal)
	}
	if page.TypeTotals[domain.DocPlan] != 3 || len(page.TypeTotals) != 1 {
		t.Fatalf("library type facets = %#v, want plan:3", page.TypeTotals)
	}
	if len(page.TagTotals) != 1 || page.TagTotals[0] != (domain.TagCount{Tag: "ops", Count: 3}) {
		t.Fatalf("library tag facets = %#v, want ops:3", page.TagTotals)
	}
	if len(page.Documents) != 2 {
		t.Fatalf("library page len=%d, want 2: %+v", len(page.Documents), page.Documents)
	}
	for _, doc := range page.Documents {
		if doc.OwnerID != "u-library" || doc.Type != domain.DocPlan || len(doc.Tags) != 1 || doc.Tags[0] != "ops" {
			t.Fatalf("library page escaped owner/type/tag filter: %+v", doc)
		}
	}

	archived, err := store.ListLibraryPage(ctx, "u-library", ports.DocumentLibraryQuery{
		NodeIDs: []string{"lib-n1", "lib-n2"}, FilterNodeIDs: true, Types: []domain.DocumentType{domain.DocPlan},
		Tags: []string{"ops"}, Status: ports.DocumentLibraryArchived, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if archived.Total != 1 || len(archived.Documents) != 1 || archived.Documents[0].ID != "lib-n2-archived" {
		t.Fatalf("archived library page = %+v", archived)
	}
	if archived.TypeTotals[domain.DocPlan] != 1 || len(archived.TagTotals) != 1 || archived.TagTotals[0].Count != 1 {
		t.Fatalf("archived contextual facets = %+v", archived)
	}
	empty, err := store.ListLibraryPage(ctx, "u-library", ports.DocumentLibraryQuery{
		FilterNodeIDs: true, Status: ports.DocumentLibraryAll, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Total != 0 || empty.ActiveTotal != 0 || empty.ArchivedTotal != 0 || len(empty.Documents) != 0 {
		t.Fatalf("empty resolved node set must match nothing, got %+v", empty)
	}

	search, err := store.ListLibraryPage(ctx, "u-library", ports.DocumentLibraryQuery{
		NodeIDs: []string{"lib-n1", "lib-n2"}, FilterNodeIDs: true, Types: []domain.DocumentType{domain.DocPlan},
		Tags: []string{"ops"}, Status: ports.DocumentLibraryAll, Search: "alpha", Limit: 1, Offset: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 3 || search.ActiveTotal != 2 || search.ArchivedTotal != 1 || len(search.Results) != 1 {
		t.Fatalf("paginated library search = %+v, want total 3, facets 2/1 and one hit", search)
	}
	if search.Results[0].ID != "lib-n2-archived" || !strings.Contains(search.Results[0].Snippet, domain.HighlightStart) {
		t.Fatalf("ranked library search page = %+v, want highlighted lib-n2-archived", search.Results)
	}

	toggleDone := make(chan error, 1)
	go func() {
		for i := 0; i < 50; i++ {
			if err := store.SetArchived(ctx, "u-library", "lib-n2-archived", i%2 == 0); err != nil {
				toggleDone <- err
				return
			}
		}
		toggleDone <- nil
	}()
	for i := 0; i < 50; i++ {
		snapshot, err := store.ListLibraryPage(ctx, "u-library", ports.DocumentLibraryQuery{
			NodeIDs: []string{"lib-n1", "lib-n2"}, FilterNodeIDs: true, Types: []domain.DocumentType{domain.DocPlan},
			Tags: []string{"ops"}, Status: ports.DocumentLibraryAll, Search: "alpha", Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		archivedDocs := 0
		for _, hit := range snapshot.Results {
			if hit.Archived {
				archivedDocs++
			}
		}
		if snapshot.Total != 3 || len(snapshot.Results) != 3 || snapshot.ActiveTotal+snapshot.ArchivedTotal != 3 || archivedDocs != snapshot.ArchivedTotal || snapshot.TypeTotals[domain.DocPlan] != 3 || len(snapshot.TagTotals) != 1 || snapshot.TagTotals[0].Count != 3 {
			t.Fatalf("mixed library snapshot during concurrent archive: %+v archivedDocs=%d", snapshot, archivedDocs)
		}
	}
	if err := <-toggleDone; err != nil {
		t.Fatal(err)
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
	if _, err := st.Create(ctx, domain.Document{
		ID: "srch-memory", OwnerID: owner, Type: domain.DocMemory, Path: "memory-hit",
		Title: "Andere Erinnerung", Body: "kompendium steht nur im body", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := st.Search(ctx, owner, "kompend", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "a" {
		t.Fatalf(`search "kompend" = %#v, want a first plus the memory regression fixture`, hits)
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
	typed, err := st.SearchQuery(ctx, owner, ports.DocumentSearchQuery{Text: "kompendium", Type: domain.DocMemory, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(typed) != 1 || typed[0].ID != "srch-memory" {
		t.Fatalf("typed limited keyword search = %#v, want memory hit", typed)
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
	if _, err := s.Create(ctx, domain.Document{ID: "memory", OwnerID: owner, Type: domain.DocMemory, Path: "memory", Title: "Memory", Body: "memory doc", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	if err := s.ReplaceChunks(ctx, "near", owner, snapshotHash(t, s, "near"), []string{"near chunk"}, [][]float32{vec(0.9)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "far", owner, snapshotHash(t, s, "far"), []string{"far chunk"}, [][]float32{vec(-0.9)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "memory", owner, snapshotHash(t, s, "memory"), []string{"memory chunk"}, [][]float32{vec(-0.5)}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.SemanticSearch(ctx, owner, vec(1.0), nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].Path != "near" {
		t.Fatalf("want near first, got %#v", hits)
	}
	if hits[0].Snippet != "near chunk" {
		t.Fatalf("snippet = %q, want near chunk", hits[0].Snippet)
	}
	tagged, _ := s.SemanticSearch(ctx, owner, vec(1.0), nil, []string{"go"}, 10)
	if len(tagged) != 1 || tagged[0].Path != "near" {
		t.Fatalf("tag-filtered semantic = %#v, want [near]", tagged)
	}
	typed, err := s.SemanticSearchQuery(ctx, owner, vec(1.0), ports.DocumentSearchQuery{Type: domain.DocMemory, Limit: 1})
	if err != nil || len(typed) != 1 || typed[0].ID != "memory" {
		t.Fatalf("typed limited semantic search = %#v, %v; want memory hit", typed, err)
	}
	if err := s.SetArchived(ctx, owner, "near", true); err != nil {
		t.Fatal(err)
	}
	library, err := s.ListLibraryPage(ctx, owner, ports.DocumentLibraryQuery{
		Search: "no-keyword-match", Embedding: vec(1.0), Status: ports.DocumentLibraryAll, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if library.Total != 3 || library.ActiveTotal != 2 || library.ArchivedTotal != 1 || len(library.Results) != 3 {
		t.Fatalf("semantic-only library search = %+v, want active+archived result set", library)
	}
	if library.Results[0].ID != "near" || !library.Results[0].Archived || library.Results[0].Snippet != "near chunk" {
		t.Fatalf("semantic-only top library hit = %+v, want archived near chunk", library.Results[0])
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

	if err := docs.SetArchived(ctx, "u1", "d1", true); err != nil {
		t.Fatal(err)
	}
	archived, err := docs.ListArchived(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || len(archived[0].Tags) != 2 {
		t.Fatalf("ListArchived must hydrate tags for archive filters, got %+v", archived)
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

func TestDocumentStore_ReorderPrioritiesRollsBackForeignID(t *testing.T) {
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	seedUser(t, us, "u2")
	for _, d := range []domain.Document{
		{ID: "a", OwnerID: "u1", Type: domain.DocMemory, Path: "a", Title: "A", Priority: 7},
		{ID: "foreign", OwnerID: "u2", Type: domain.DocMemory, Path: "foreign", Title: "Foreign"},
	} {
		if _, err := ds.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := ds.ReorderPriorities(ctx, "u1", []string{"a", "foreign"}); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("error = %v, want ErrDocumentNotFound", err)
	}
	got, err := ds.Get(ctx, "u1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != 7 {
		t.Fatalf("partial reorder survived rollback: priority=%d", got.Priority)
	}
}

func TestDocumentStore_ConcurrentReorderPrioritiesKeepsCompleteOrder(t *testing.T) {
	ds, us, _, done := newDocStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	for _, id := range []string{"a", "b", "c"} {
		if _, err := ds.Create(ctx, domain.Document{ID: id, OwnerID: "u1", Type: domain.DocMemory, Path: id, Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, order := range [][]string{{"a", "b", "c"}, {"c", "b", "a"}} {
		order := append([]string(nil), order...)
		go func() {
			<-start
			errs <- ds.ReorderPriorities(ctx, "u1", order)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent reorder: %v", err)
		}
	}
	priorities := map[string]int{}
	for _, id := range []string{"a", "b", "c"} {
		d, err := ds.Get(ctx, "u1", id)
		if err != nil {
			t.Fatal(err)
		}
		priorities[id] = d.Priority
	}
	forward := priorities["a"] == 3 && priorities["b"] == 2 && priorities["c"] == 1
	reverse := priorities["c"] == 3 && priorities["b"] == 2 && priorities["a"] == 1
	if !forward && !reverse {
		t.Fatalf("concurrent reorder mixed two orders: %+v", priorities)
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

func TestDocumentStore_CurateDocumentsIsAtomicOwnerScopedAndRaceConsistent(t *testing.T) {
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
	curator, _ := domain.NewUser("curator", "sub-curator", "curator", "curator@example.invalid", "Curator")
	other, _ := domain.NewUser("other", "sub-other-curator", "other-curator", "other@example.invalid", "Other")
	for _, user := range []domain.User{curator, other} {
		if _, err := users.UpsertBySub(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "curate-1", OwnerID: "curator", Type: domain.DocMemory, Path: "memory/one", Title: "One", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "curate-2", OwnerID: "curator", Type: domain.DocMemory, Path: "memory/two", Title: "Two", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "curate-free", OwnerID: "curator", Type: domain.DocFree, Path: "free/one", Title: "Free", CreatedAt: now, UpdatedAt: now},
		{ID: "curate-foreign", OwnerID: "other", Type: domain.DocMemory, Path: "memory/foreign", Title: "Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	archived := true
	_, err = docs.CurateDocuments(ctx, "curator", ports.DocumentCurationMutation{
		IDs: []string{"curate-1", "curate-foreign"}, Archived: &archived,
		ActorKind: "human", ActorRef: "Soenne", At: now.Add(time.Hour),
	})
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("mixed-owner mutation error = %v, want ErrDocumentNotFound", err)
	}
	unchanged, err := docs.Get(ctx, "curator", "curate-1")
	if err != nil || unchanged.Archived || !unchanged.Pinned {
		t.Fatalf("mixed-owner mutation escaped rollback: %+v err=%v", unchanged, err)
	}
	mode := domain.ContextModeImmer
	_, err = docs.CurateDocuments(ctx, "curator", ports.DocumentCurationMutation{
		IDs: []string{"curate-1", "curate-free"}, ContextMode: &mode,
		ActorKind: "human", ActorRef: "Soenne", At: now.Add(time.Hour),
	})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("mixed context types error = %v, want ErrInvalidDocument", err)
	}
	unchanged, err = docs.Get(ctx, "curator", "curate-1")
	if err != nil || unchanged.ContextMode.OrAuto() != domain.ContextModeAuto {
		t.Fatalf("mixed context types escaped rollback: %+v err=%v", unchanged, err)
	}

	changed, err := docs.CurateDocuments(ctx, "curator", ports.DocumentCurationMutation{
		IDs: []string{"curate-2", "curate-1"}, Archived: &archived,
		ActorKind: "human", ActorRef: "Soenne", At: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 || changed[0].ID != "curate-2" || changed[1].ID != "curate-1" {
		t.Fatalf("curation result order = %+v", changed)
	}
	for _, doc := range changed {
		if !doc.Archived || doc.Pinned || doc.ArchivedAt == nil || !doc.ArchivedAt.Equal(now.Add(time.Hour)) || doc.UpdatedByKind != "human" || doc.UpdatedByRef != "Soenne" {
			t.Fatalf("archive state/provenance incomplete: %+v", doc)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			value := i%2 == 0
			if _, err := docs.CurateDocuments(ctx, "curator", ports.DocumentCurationMutation{
				IDs: []string{"curate-1", "curate-2"}, Archived: &value,
				ActorKind: "agent", ActorRef: "race", At: now.Add(time.Duration(i+2) * time.Hour),
			}); err != nil {
				t.Errorf("concurrent curation %d: %v", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			mode := domain.ContextModeImmer
			if i%2 == 0 {
				mode = domain.ContextModeNie
			}
			if _, err := docs.CurateDocuments(ctx, "curator", ports.DocumentCurationMutation{
				IDs: []string{"curate-1", "curate-2"}, ContextMode: &mode,
				ActorKind: "agent", ActorRef: "race", At: now.Add(time.Duration(i+2) * time.Hour),
			}); err != nil {
				t.Errorf("concurrent context curation %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	one, err := docs.Get(ctx, "curator", "curate-1")
	if err != nil {
		t.Fatal(err)
	}
	two, err := docs.Get(ctx, "curator", "curate-2")
	if err != nil {
		t.Fatal(err)
	}
	if one.Archived != two.Archived ||
		(one.ArchivedAt == nil) != (two.ArchivedAt == nil) ||
		(one.ArchivedAt != nil && !one.ArchivedAt.Equal(*two.ArchivedAt)) ||
		!one.UpdatedAt.Equal(two.UpdatedAt) ||
		one.ContextMode != two.ContextMode ||
		one.UpdatedByRef != two.UpdatedByRef {
		t.Fatalf("concurrent batch tore aggregate state: one=%+v two=%+v", one, two)
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
