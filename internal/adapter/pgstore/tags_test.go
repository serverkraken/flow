package pgstore_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func newTagStore(t *testing.T) (*pgstore.TagStore, *pgstore.UserStore, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pgstore.NewTagStore(pool, &testutil.FakeIDGen{}), pgstore.NewUserStore(pool), func() { pool.Close() }
}

func seedUser(t *testing.T, us *pgstore.UserStore, id string) {
	t.Helper()
	u, err := domain.NewUser(id, "sub-"+id, "u"+id, id+"@test.de", "User "+id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := us.UpsertBySub(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func TestTagStore_SetTagsThenFilterAndHydrate(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Go", "tui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.SetTags(ctx, "u1", domain.TaggableDocument, "d2", []string{"go"}); err != nil {
		t.Fatal(err)
	}

	// AND filter: only d1 has both go+tui.
	ids, err := ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go", "tui"}, domain.TagMatchAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("AND want [d1], got %v", ids)
	}

	// OR filter: both d1+d2 have go.
	ids, _ = ts.FilterIDs(ctx, "u1", domain.TaggableDocument, []string{"go"}, domain.TagMatchAny)
	if len(ids) != 2 {
		t.Fatalf("OR want 2, got %v", ids)
	}

	// Hydrate.
	m, err := ts.TagsForMany(ctx, "u1", domain.TaggableDocument, []string{"d1", "d2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m["d1"]) != 2 || len(m["d2"]) != 1 {
		t.Fatalf("hydrate mismatch: %+v", m)
	}
}

func TestTagStore_SetReplacesDiff_AndDisplayFirstWriteWins(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")

	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"Django"}) // display "Django"
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"django", "postgres"})

	got, _ := ts.TagsFor(ctx, "u1", domain.TaggableDocument, "d1")
	if len(got) != 2 {
		t.Fatalf("want 2 tags, got %+v", got)
	}
	for _, tg := range got {
		if tg.Slug == "django" && tg.Display != "Django" {
			t.Errorf("display should be first-write-wins 'Django', got %q", tg.Display)
		}
	}
}

func TestTagStore_ListTags_TypeScopeAndMerge(t *testing.T) {
	t.Parallel()
	ts, us, done := newTagStore(t)
	defer done()
	ctx := context.Background()
	seedUser(t, us, "u1")
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableDocument, "d1", []string{"go"})
	_, _ = ts.SetTags(ctx, "u1", domain.TaggableWorkSession, "s1", []string{"go", "deep"})

	docType := domain.TaggableDocument
	tc, err := ts.ListTags(ctx, "u1", domain.TagScope{Type: &docType})
	if err != nil {
		t.Fatal(err)
	}
	if len(tc) != 1 || tc[0].Tag != "go" || tc[0].Count != 1 {
		t.Fatalf("doc-scoped ListTags want [{go,1}], got %+v", tc)
	}

	// Merge deep→go across everything.
	if err := ts.MergeTags(ctx, "u1", "deep", "go"); err != nil {
		t.Fatal(err)
	}
	all, _ := ts.ListTags(ctx, "u1", domain.TagScope{})
	for _, c := range all {
		if c.Tag == "deep" {
			t.Errorf("deep should be merged away, got %+v", all)
		}
	}
}

func TestTagStore_BackfillFromLegacyColumns(t *testing.T) {
	t.Parallel()
	// Migrate up to 0018 (pre-tags), insert a doc with tags[] + a session with a tag,
	// then run Migrate (0019 backfill) and assert taggings exist.
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pgstore.MigrateUpTo(ctx, pool, 18); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO users (id, oidc_sub, username, created_at) VALUES ('u1','s','u',now())`)
	mustExec(t, pool, `INSERT INTO documents (id,owner_id,node_id,type,path,title,body,tags,extra,created_at,updated_at)
		VALUES ('d1','u1',NULL,'free','p','t','body',ARRAY['go','tui'],'{}',now(),now())`)
	mustExec(t, pool, `INSERT INTO work_sessions (id,owner_id,node_id,tag,note,start_at,created_at)
		VALUES ('s1','u1',NULL,'Deep','n',now(),now())`)

	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	ts := pgstore.NewTagStore(pool, &testutil.FakeIDGen{})
	got, _ := ts.TagsFor(ctx, "u1", domain.TaggableDocument, "d1")
	if len(got) != 2 {
		t.Fatalf("backfilled doc tags want 2, got %+v", got)
	}
	st, _ := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if len(st) != 1 || st[0].Slug != "deep" {
		t.Fatalf("backfilled session tag want [deep], got %+v", st)
	}
}
