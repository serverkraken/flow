package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func newArtifactStore(t *testing.T) (*pgstore.ArtifactStore, *pgstore.UserStore, *pgstore.NodeStore, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pgstore.NewArtifactStore(pool), pgstore.NewUserStore(pool), pgstore.NewNodeStore(pool), func() { pool.Close() }
}

func seedArtifactNode(t *testing.T, ns *pgstore.NodeStore, ownerID, nodeID string, parentID *string, kind domain.NodeKind) domain.Node {
	t.Helper()
	n, err := domain.NewNode(nodeID, ownerID, "Node "+nodeID, nodeID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = kind
	n.ParentID = parentID
	got, err := ns.Create(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestArtifactStore_PutGetRoundTrip(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Artifact{
		ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "bild", Name: "bild.png",
		Mime: "image/png", SizeBytes: 3, Ref: "abc123def456", Bytes: []byte{1, 2, 3},
		Width: 64, Height: 32, CreatedByKind: "human", CreatedByRef: "u1",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := as.Get(ctx, "u1", n.ID, "bild")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bytes) != 3 || got.Bytes[0] != 1 {
		t.Fatalf("bytes did not round-trip: %+v", got.Bytes)
	}
	if got.Name != "bild.png" || got.Mime != "image/png" || got.Ref != "abc123def456" {
		t.Errorf("meta did not round-trip: %+v", got)
	}
	if got.Width != 64 || got.Height != 32 {
		t.Errorf("dimensions did not round-trip: %dx%d", got.Width, got.Height)
	}
}

func TestArtifactStore_GetMetaExcludesBytes(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	a := domain.Artifact{
		ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "doc", Name: "doc.pdf",
		Mime: "application/pdf", SizeBytes: 5, Ref: "ref123456789", Bytes: []byte{1, 2, 3, 4, 5},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}
	meta, err := as.GetMeta(ctx, "u1", n.ID, "doc")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Bytes != nil {
		t.Errorf("GetMeta must not return bytes, got %v", meta.Bytes)
	}
	if meta.Name != "doc.pdf" || meta.SizeBytes != 5 {
		t.Errorf("meta fields wrong: %+v", meta)
	}
}

func TestArtifactStore_ListAcrossAncestorChainNewestFirst(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	eng := seedArtifactNode(t, ns, "u1", "eng", nil, domain.KindEngagement)
	repo := seedArtifactNode(t, ns, "u1", "repo", &eng.ID, domain.KindRepo)

	t0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	older := domain.Artifact{ID: "a-old", OwnerID: "u1", NodeID: eng.ID, Slug: "old", Name: "old.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref000000001", Bytes: []byte{1}, CreatedAt: t0, UpdatedAt: t0}
	newer := domain.Artifact{ID: "a-new", OwnerID: "u1", NodeID: repo.ID, Slug: "new", Name: "new.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref000000002", Bytes: []byte{2}, CreatedAt: t0.Add(time.Minute), UpdatedAt: t0.Add(time.Minute)}
	if err := as.Put(ctx, older); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, newer); err != nil {
		t.Fatal(err)
	}

	list, err := as.List(ctx, "u1", repo.ID, eng.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 artifacts across ancestor chain, got %d: %+v", len(list), list)
	}
	if list[0].Slug != "new" || list[1].Slug != "old" {
		t.Errorf("want newest-first [new,old], got [%s,%s]", list[0].Slug, list[1].Slug)
	}
	if list[0].Bytes != nil {
		t.Errorf("List must return meta only (no bytes), got %v", list[0].Bytes)
	}
}

func TestArtifactStore_ExistingSlugs(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	for _, slug := range []string{"a", "b", "c"} {
		a := domain.Artifact{ID: "id-" + slug, OwnerID: "u1", NodeID: n.ID, Slug: slug, Name: slug,
			Mime: "text/plain", SizeBytes: 1, Ref: "ref00000000" + slug, Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
		if err := as.Put(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	slugs, err := as.ExistingSlugs(ctx, "u1", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 3 {
		t.Fatalf("want 3 slugs, got %+v", slugs)
	}
}

func TestArtifactStore_TotalBytesSums(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	items := []struct {
		slug string
		size int64
		ref  string
	}{
		{"sa", 100, "ref00000000a"},
		{"sb", 200, "ref00000000b"},
		{"sc", 300, "ref00000000c"},
	}
	for _, it := range items {
		a := domain.Artifact{ID: "id-" + it.slug, OwnerID: "u1", NodeID: n.ID, Slug: it.slug, Name: "n",
			Mime: "text/plain", SizeBytes: it.size, Ref: it.ref, Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
		if err := as.Put(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	total, err := as.TotalBytes(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if total != 600 {
		t.Errorf("want total 600, got %d", total)
	}
}

func TestArtifactStore_DeleteNotFound(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	a := domain.Artifact{ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "bild", Name: "bild.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref123456789", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := as.Delete(ctx, "u1", n.ID, "bild"); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Get(ctx, "u1", n.ID, "bild"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("want ErrArtifactNotFound after delete, got %v", err)
	}
	if err := as.Delete(ctx, "u1", n.ID, "bild"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("delete of already-absent artifact: want ErrArtifactNotFound, got %v", err)
	}
}

// TestArtifactStore_OwnerScopeNegative verifies that a foreign owner cannot
// read, list, or delete another owner's artifact — the multi-tenant
// invariant that must hold for every new data surface (AGENTS.md Grundsätze).
func TestArtifactStore_OwnerScopeNegative(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	seedUser(t, us, "intruder")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	a := domain.Artifact{ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "bild", Name: "bild.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref123456789", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}

	if _, err := as.Get(ctx, "intruder", n.ID, "bild"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner Get: want ErrArtifactNotFound, got %v", err)
	}
	if _, err := as.GetMeta(ctx, "intruder", n.ID, "bild"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner GetMeta: want ErrArtifactNotFound, got %v", err)
	}
	list, err := as.List(ctx, "intruder", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("foreign owner List: want empty, got %+v", list)
	}
	if err := as.Delete(ctx, "intruder", n.ID, "bild"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner Delete: want ErrArtifactNotFound, got %v", err)
	}
	if err := as.Rename(ctx, "intruder", n.ID, "bild", "renamed"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner Rename: want ErrArtifactNotFound, got %v", err)
	}
	// The real owner must still see the untouched artifact.
	got, err := as.Get(ctx, "u1", n.ID, "bild")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "bild.png" {
		t.Errorf("owner's artifact must survive foreign-owner attempts, got %+v", got)
	}
}

func TestArtifactStore_PutUpsertsOnSlugCollision(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	first := domain.Artifact{ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "bild", Name: "bild.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref111111111", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Bytes = []byte{9, 9, 9}
	second.SizeBytes = 3
	second.Ref = "ref222222222"
	second.UpdatedAt = now.Add(time.Minute)
	if err := as.Put(ctx, second); err != nil {
		t.Fatal(err)
	}

	got, err := as.Get(ctx, "u1", n.ID, "bild")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bytes) != 3 || got.Bytes[0] != 9 || got.Ref != "ref222222222" {
		t.Fatalf("upsert did not overwrite: %+v", got)
	}
	slugs, err := as.ExistingSlugs(ctx, "u1", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 {
		t.Fatalf("upsert must not duplicate: %+v", slugs)
	}
}

func TestArtifactStore_Rename(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	a := domain.Artifact{ID: "a1", OwnerID: "u1", NodeID: n.ID, Slug: "bild", Name: "bild.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref123456789", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := as.Rename(ctx, "u1", n.ID, "bild", "Neuer Name.png"); err != nil {
		t.Fatal(err)
	}
	got, err := as.Get(ctx, "u1", n.ID, "bild")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Neuer Name.png" {
		t.Errorf("rename did not take effect: %+v", got)
	}
	if got.Slug != "bild" || got.Ref != "ref123456789" || len(got.Bytes) != 1 {
		t.Errorf("rename must not touch slug/ref/bytes: %+v", got)
	}
	if err := as.Rename(ctx, "u1", n.ID, "nope", "x"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("rename of missing artifact: want ErrArtifactNotFound, got %v", err)
	}
}

// --- Free (node-less) artifacts — owner-global library, node_id NULL ---

func TestArtifactStore_FreePutGetRoundTrip(t *testing.T) {
	t.Parallel()
	as, us, _, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Artifact{
		ID: "a1", OwnerID: "u1", NodeID: "", Slug: "logo", Name: "logo.png",
		Mime: "image/png", SizeBytes: 3, Ref: "abc123def456", Bytes: []byte{1, 2, 3},
		Width: 64, Height: 32, CreatedByKind: "human", CreatedByRef: "u1",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := as.Get(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeID != "" {
		t.Errorf("free artifact Get: want NodeID==\"\", got %q", got.NodeID)
	}
	if len(got.Bytes) != 3 || got.Bytes[0] != 1 {
		t.Fatalf("bytes did not round-trip: %+v", got.Bytes)
	}

	meta, err := as.GetMeta(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Bytes != nil {
		t.Errorf("GetMeta must not return bytes, got %v", meta.Bytes)
	}
	if meta.NodeID != "" {
		t.Errorf("free artifact GetMeta: want NodeID==\"\", got %q", meta.NodeID)
	}

	if err := as.Rename(ctx, "u1", "", "logo", "Neues Logo.png"); err != nil {
		t.Fatal(err)
	}
	renamed, err := as.Get(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Neues Logo.png" {
		t.Errorf("free rename did not take effect: %+v", renamed)
	}
	if err := as.Rename(ctx, "u1", "", "nope", "x"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("rename of missing free artifact: want ErrArtifactNotFound, got %v", err)
	}

	if err := as.Delete(ctx, "u1", "", "logo"); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Get(ctx, "u1", "", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("want ErrArtifactNotFound after free delete, got %v", err)
	}
	if err := as.Delete(ctx, "u1", "", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("2nd delete of already-absent free artifact: want ErrArtifactNotFound, got %v", err)
	}
}

func TestArtifactStore_ListFreeOnlyNodeLessNewestFirst(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	t0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	freeOld := domain.Artifact{ID: "f-old", OwnerID: "u1", NodeID: "", Slug: "free-old", Name: "old.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref000000001", Bytes: []byte{1}, CreatedAt: t0, UpdatedAt: t0}
	freeNew := domain.Artifact{ID: "f-new", OwnerID: "u1", NodeID: "", Slug: "free-new", Name: "new.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref000000002", Bytes: []byte{2}, CreatedAt: t0.Add(time.Minute), UpdatedAt: t0.Add(time.Minute)}
	nodeBound := domain.Artifact{ID: "n-bound", OwnerID: "u1", NodeID: n.ID, Slug: "node-one", Name: "node.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref000000003", Bytes: []byte{3}, CreatedAt: t0.Add(2 * time.Minute), UpdatedAt: t0.Add(2 * time.Minute)}
	for _, a := range []domain.Artifact{freeOld, freeNew, nodeBound} {
		if err := as.Put(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	list, err := as.ListFree(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 free artifacts, got %d: %+v", len(list), list)
	}
	if list[0].Slug != "free-new" || list[1].Slug != "free-old" {
		t.Errorf("want newest-first [free-new,free-old], got [%s,%s]", list[0].Slug, list[1].Slug)
	}
	for _, a := range list {
		if a.NodeID != "" {
			t.Errorf("ListFree must only return node-less artifacts, got NodeID=%q", a.NodeID)
		}
		if a.Bytes != nil {
			t.Errorf("ListFree must return meta only (no bytes), got %v", a.Bytes)
		}
	}

	// Owner-scope negative.
	seedUser(t, us, "intruder")
	foreign, err := as.ListFree(ctx, "intruder")
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign) != 0 {
		t.Errorf("foreign owner ListFree: want empty, got %+v", foreign)
	}
}

func TestArtifactStore_ExistingSlugsFreeOnly(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	free := domain.Artifact{ID: "f1", OwnerID: "u1", NodeID: "", Slug: "free-a", Name: "a",
		Mime: "text/plain", SizeBytes: 1, Ref: "ref00000000a", Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
	nodeBound := domain.Artifact{ID: "n1", OwnerID: "u1", NodeID: n.ID, Slug: "node-b", Name: "b",
		Mime: "text/plain", SizeBytes: 1, Ref: "ref00000000b", Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, free); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, nodeBound); err != nil {
		t.Fatal(err)
	}

	slugs, err := as.ExistingSlugs(ctx, "u1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "free-a" {
		t.Fatalf("want only free slug [free-a], got %+v", slugs)
	}
}

// TestArtifactStore_FreeOwnerScopeNegative mirrors
// TestArtifactStore_OwnerScopeNegative for the node-less (free) path — a
// foreign owner must not read, rename, delete, or enumerate another owner's
// free artifact via the NULL-safe (IS NOT DISTINCT FROM) queries.
func TestArtifactStore_FreeOwnerScopeNegative(t *testing.T) {
	t.Parallel()
	as, us, _, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	seedUser(t, us, "intruder")

	now := time.Now().UTC()
	a := domain.Artifact{ID: "a1", OwnerID: "u1", NodeID: "", Slug: "logo", Name: "logo.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref123456789", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, a); err != nil {
		t.Fatal(err)
	}

	if _, err := as.Get(ctx, "intruder", "", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner free Get: want ErrArtifactNotFound, got %v", err)
	}
	if _, err := as.GetMeta(ctx, "intruder", "", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner free GetMeta: want ErrArtifactNotFound, got %v", err)
	}
	if err := as.Rename(ctx, "intruder", "", "logo", "renamed"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner free Rename: want ErrArtifactNotFound, got %v", err)
	}
	if err := as.Delete(ctx, "intruder", "", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("foreign owner free Delete: want ErrArtifactNotFound, got %v", err)
	}
	slugs, err := as.ExistingSlugs(ctx, "intruder", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("foreign owner free ExistingSlugs: want empty, got %+v", slugs)
	}

	// The real owner must still see the untouched artifact.
	got, err := as.Get(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "logo.png" {
		t.Errorf("owner's free artifact must survive foreign-owner attempts, got %+v", got)
	}
}

// TestArtifactStore_FreeAndNodeSlugCoexist verifies the partial-unique index
// scopes to node_id IS NULL only — a free "logo" and a node-bound "logo" for
// the same owner do not collide (the pre-existing UNIQUE(owner,node,slug)
// never fires on NULL rows; the new partial index only fires among free rows).
func TestArtifactStore_FreeAndNodeSlugCoexist(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	free := domain.Artifact{ID: "f1", OwnerID: "u1", NodeID: "", Slug: "logo", Name: "free-logo.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref111111111", Bytes: []byte{1}, CreatedAt: now, UpdatedAt: now}
	nodeBound := domain.Artifact{ID: "n1", OwnerID: "u1", NodeID: n.ID, Slug: "logo", Name: "node-logo.png",
		Mime: "image/png", SizeBytes: 1, Ref: "ref222222222", Bytes: []byte{2}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, free); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, nodeBound); err != nil {
		t.Fatal(err)
	}

	gotFree, err := as.Get(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if gotFree.Name != "free-logo.png" {
		t.Errorf("free logo wrong: %+v", gotFree)
	}
	gotNode, err := as.Get(ctx, "u1", n.ID, "logo")
	if err != nil {
		t.Fatal(err)
	}
	if gotNode.Name != "node-logo.png" {
		t.Errorf("node logo wrong: %+v", gotNode)
	}

	// A second free Put on the same slug must overwrite (ON CONFLICT on the
	// partial index), not duplicate — one row, updated fields.
	second := free
	second.ID = "f2"
	second.Bytes = []byte{9, 9, 9}
	second.SizeBytes = 3
	second.Ref = "ref333333333"
	second.UpdatedAt = now.Add(time.Minute)
	if err := as.Put(ctx, second); err != nil {
		t.Fatal(err)
	}
	gotFree2, err := as.Get(ctx, "u1", "", "logo")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotFree2.Bytes) != 3 || gotFree2.Bytes[0] != 9 || gotFree2.Ref != "ref333333333" {
		t.Fatalf("2nd free Put must overwrite, not duplicate: %+v", gotFree2)
	}
	freeSlugs, err := as.ExistingSlugs(ctx, "u1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSlugs) != 1 {
		t.Fatalf("free upsert must not duplicate: %+v", freeSlugs)
	}
}

func TestArtifactStore_TotalBytesSumsNodeAndFree(t *testing.T) {
	t.Parallel()
	as, us, ns, done := newArtifactStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	n := seedArtifactNode(t, ns, "u1", "n1", nil, domain.KindEngagement)

	now := time.Now().UTC()
	nodeArt := domain.Artifact{ID: "n1", OwnerID: "u1", NodeID: n.ID, Slug: "node-file", Name: "n",
		Mime: "text/plain", SizeBytes: 100, Ref: "ref00000000a", Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
	freeArt := domain.Artifact{ID: "f1", OwnerID: "u1", NodeID: "", Slug: "free-file", Name: "f",
		Mime: "text/plain", SizeBytes: 250, Ref: "ref00000000b", Bytes: []byte{0}, CreatedAt: now, UpdatedAt: now}
	if err := as.Put(ctx, nodeArt); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(ctx, freeArt); err != nil {
		t.Fatal(err)
	}

	total, err := as.TotalBytes(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if total != 350 {
		t.Errorf("want total 350 (node+free), got %d", total)
	}
}
