package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func newDocumentAggregateFixture(t *testing.T) (context.Context, *pgstore.DocumentStore, *pgstore.TagStore, func(string, ...any)) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-doc-agg", "sub-doc-agg", "doc-agg", "doc-agg@example.test", "Document Aggregate")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	ids := &testutil.FakeIDGen{}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, pgstore.NewDocumentStore(pool, ids), pgstore.NewTagStore(pool, ids), exec
}

func aggregateDocument(id, path, title, body string, now time.Time) domain.Document {
	return domain.Document{
		ID: id, OwnerID: "u-doc-agg", Type: domain.DocFree, Path: path,
		Title: title, Body: body, CreatedAt: now, UpdatedAt: now,
	}
}

func TestDocumentAggregateStore_RollsBackFollowFailures(t *testing.T) {
	ctx, docs, tags, exec := newDocumentAggregateFixture(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	oldTags := []string{"old"}
	for _, id := range []string{"doc-link", "doc-tag", "doc-delete", "doc-upsert"} {
		if _, err := docs.CreateDocumentAggregate(ctx,
			aggregateDocument(id, "notes/"+id, "Before", "[[before]]", now),
			ports.DocumentAggregateChanges{Links: []string{"before"}, Tags: &oldTags}); err != nil {
			t.Fatal(err)
		}
	}

	exec(`CREATE FUNCTION test_fail_document_link() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN IF NEW.src_doc_id IN ('doc-link','doc-create-link','doc-upsert') THEN RAISE EXCEPTION 'link failure'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER test_fail_document_link BEFORE INSERT ON document_links
FOR EACH ROW EXECUTE FUNCTION test_fail_document_link()`)
	exec(`CREATE FUNCTION test_fail_document_tag() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.taggable_id IN ('doc-tag','doc-create-tag') THEN RAISE EXCEPTION 'tag failure'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER test_fail_document_tag BEFORE INSERT ON taggings
FOR EACH ROW EXECUTE FUNCTION test_fail_document_tag()`)
	exec(`CREATE FUNCTION test_fail_document_tag_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF OLD.taggable_id='doc-delete' THEN RAISE EXCEPTION 'tag delete failure'; END IF; RETURN OLD; END $$`)
	exec(`CREATE TRIGGER test_fail_document_tag_delete BEFORE DELETE ON taggings
FOR EACH ROW EXECUTE FUNCTION test_fail_document_tag_delete()`)

	newTags := []string{"new"}
	_, err := docs.UpdateDocumentAggregate(ctx, "u-doc-agg", "doc-link", func(d domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		d.Title, d.Body, d.UpdatedAt = "After", "[[after]]", now.Add(time.Hour)
		return d, ports.DocumentAggregateChanges{Links: []string{"after"}, Tags: &newTags}, nil
	})
	if err == nil {
		t.Fatal("link failure did not abort update")
	}
	_, err = docs.UpdateDocumentAggregate(ctx, "u-doc-agg", "doc-tag", func(d domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		d.Title, d.Body, d.UpdatedAt = "After", "[[after]]", now.Add(time.Hour)
		return d, ports.DocumentAggregateChanges{Links: []string{"after"}, Tags: &newTags}, nil
	})
	if err == nil {
		t.Fatal("tag failure did not abort update")
	}
	for _, id := range []string{"doc-link", "doc-tag"} {
		got, err := docs.Get(ctx, "u-doc-agg", id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title != "Before" || got.Body != "[[before]]" || len(got.Tags) != 1 || got.Tags[0] != "old" {
			t.Fatalf("%s partially changed: %+v", id, got)
		}
		backlinks, err := docs.Backlinks(ctx, "u-doc-agg", "before")
		if err != nil {
			t.Fatal(err)
		}
		if !containsDocumentID(backlinks, id) {
			t.Fatalf("%s lost old link after rollback: %+v", id, backlinks)
		}
	}
	upsertTags := []string{"upserted"}
	if _, err := docs.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
		OwnerID: "u-doc-agg", Type: domain.DocFree, Path: "notes/doc-upsert",
		Title: "After", Body: "[[after]]", Pinned: true, Archived: true,
		Changes: ports.DocumentAggregateChanges{Links: []string{"after"}, Tags: &upsertTags},
	}); err == nil {
		t.Fatal("upsert survived link failure")
	}
	upserted, err := docs.Get(ctx, "u-doc-agg", "doc-upsert")
	if err != nil {
		t.Fatal(err)
	}
	if upserted.Title != "Before" || upserted.Body != "[[before]]" || upserted.Pinned || upserted.Archived || len(upserted.Tags) != 1 || upserted.Tags[0] != "old" {
		t.Fatalf("upsert partially changed after link failure: %+v", upserted)
	}

	createTags := []string{"new"}
	if _, err := docs.CreateDocumentAggregate(ctx,
		aggregateDocument("doc-create-link", "notes/create-link", "Create", "[[after]]", now),
		ports.DocumentAggregateChanges{Links: []string{"after"}, Tags: &createTags}); err == nil {
		t.Fatal("create survived link failure")
	}
	if _, err := docs.CreateDocumentAggregate(ctx,
		aggregateDocument("doc-create-tag", "notes/create-tag", "Create", "[[after]]", now),
		ports.DocumentAggregateChanges{Links: []string{"after"}, Tags: &createTags}); err == nil {
		t.Fatal("create survived tag failure")
	}
	for _, id := range []string{"doc-create-link", "doc-create-tag"} {
		if _, err := docs.Get(ctx, "u-doc-agg", id); !errors.Is(err, ports.ErrDocumentNotFound) {
			t.Fatalf("partial create %s survived: %v", id, err)
		}
	}

	if err := docs.DeleteDocumentAggregate(ctx, "u-doc-agg", "doc-delete"); err == nil {
		t.Fatal("delete survived tag cleanup failure")
	}
	got, err := docs.Get(ctx, "u-doc-agg", "doc-delete")
	if err != nil || len(got.Tags) != 1 || got.Tags[0] != "old" {
		t.Fatalf("delete partially applied: doc=%+v err=%v", got, err)
	}
	if err := docs.DeleteDocumentAggregate(ctx, "foreign-owner", "doc-delete"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("foreign delete: want ErrDocumentNotFound, got %v", err)
	}

	gotTags, err := tags.TagsFor(ctx, "u-doc-agg", domain.TaggableDocument, "doc-delete")
	if err != nil || len(gotTags) != 1 || gotTags[0].Slug != "old" {
		t.Fatalf("delete tags changed: tags=%+v err=%v", gotTags, err)
	}
}

func TestDocumentAggregateStore_UpsertAndDeleteRoundTrip(t *testing.T) {
	ctx, docs, tags, _ := newDocumentAggregateFixture(t)
	firstTags := []string{"first"}
	first, err := docs.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
		OwnerID: "u-doc-agg", Type: domain.DocMemory, Path: "memory/aggregate",
		Title: "First", Body: "[[first]]", Pinned: true, Archived: true,
		Changes: ports.DocumentAggregateChanges{Links: []string{"first"}, Tags: &firstTags},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Archived || first.Pinned || first.ArchivedAt == nil || len(first.Tags) != 1 || first.Tags[0] != "first" {
		t.Fatalf("first upsert = %+v", first)
	}
	secondTags := []string{"second"}
	second, err := docs.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
		OwnerID: "u-doc-agg", Type: domain.DocMemory, Path: "memory/aggregate",
		Title: "Second", Body: "[[second]]", Pinned: true,
		Changes: ports.DocumentAggregateChanges{Links: []string{"second"}, Tags: &secondTags},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Archived || !second.Pinned || second.ArchivedAt != nil || len(second.Tags) != 1 || second.Tags[0] != "second" {
		t.Fatalf("second upsert = %+v", second)
	}
	oldBacklinks, err := docs.Backlinks(ctx, "u-doc-agg", "first")
	if err != nil || containsDocumentID(oldBacklinks, second.ID) {
		t.Fatalf("old link survived: backlinks=%+v err=%v", oldBacklinks, err)
	}
	newBacklinks, err := docs.Backlinks(ctx, "u-doc-agg", "second")
	if err != nil || !containsDocumentID(newBacklinks, second.ID) {
		t.Fatalf("new link missing: backlinks=%+v err=%v", newBacklinks, err)
	}

	if err := docs.DeleteDocumentAggregate(ctx, "u-doc-agg", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := docs.Get(ctx, "u-doc-agg", second.ID); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("deleted document survived: %v", err)
	}
	remainingTags, err := tags.TagsFor(ctx, "u-doc-agg", domain.TaggableDocument, second.ID)
	if err != nil || len(remainingTags) != 0 {
		t.Fatalf("deleted document taggings survived: tags=%+v err=%v", remainingTags, err)
	}
}

func TestDocumentAggregateStore_ConcurrentUpdatesKeepContentIndexesTogether(t *testing.T) {
	ctx, docs, _, _ := newDocumentAggregateFixture(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	initialTags := []string{"initial"}
	if _, err := docs.CreateDocumentAggregate(ctx,
		aggregateDocument("doc-race", "notes/race", "Initial", "[[initial]]", now),
		ports.DocumentAggregateChanges{Links: []string{"initial"}, Tags: &initialTags}); err != nil {
		t.Fatal(err)
	}

	variants := []string{"alpha", "beta"}
	concurrentNodeAggregateMutations(t, func(i int) error {
		variant := variants[i]
		tagValues := []string{variant}
		_, err := docs.UpdateDocumentAggregate(ctx, "u-doc-agg", "doc-race", func(d domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
			d.Title = variant
			d.Body = "[[" + variant + "]]"
			d.UpdatedAt = now.Add(time.Duration(i+1) * time.Minute)
			return d, ports.DocumentAggregateChanges{Links: []string{variant}, Tags: &tagValues}, nil
		})
		return err
	})

	got, err := docs.Get(ctx, "u-doc-agg", "doc-race")
	if err != nil {
		t.Fatal(err)
	}
	variant := got.Title
	if variant != "alpha" && variant != "beta" {
		t.Fatalf("unexpected winning variant: %+v", got)
	}
	if got.Body != "[["+variant+"]]" || len(got.Tags) != 1 || got.Tags[0] != variant {
		t.Fatalf("content and tags diverged: %+v", got)
	}
	backlinks, err := docs.Backlinks(ctx, "u-doc-agg", variant)
	if err != nil || !containsDocumentID(backlinks, got.ID) {
		t.Fatalf("content and links diverged: variant=%s backlinks=%+v err=%v", variant, backlinks, err)
	}
}

func TestDocumentAggregateStore_ConcurrentPathCreatesHaveOneConsistentWinner(t *testing.T) {
	ctx, docs, _, _ := newDocumentAggregateFixture(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	variants := []string{"alpha", "beta"}
	start := make(chan struct{})
	errCh := make(chan error, 2)
	for i := range variants {
		i := i
		go func() {
			<-start
			variant := variants[i]
			tagValues := []string{variant}
			_, err := docs.CreateDocumentAggregate(ctx,
				aggregateDocument("doc-create-"+variant, "notes/shared", variant, "[["+variant+"]]", now),
				ports.DocumentAggregateChanges{Links: []string{variant}, Tags: &tagValues})
			errCh <- err
		}()
	}
	close(start)
	var successes, collisions int
	for range variants {
		err := <-errCh
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ports.ErrDocumentExists):
			collisions++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || collisions != 1 {
		t.Fatalf("successes=%d collisions=%d, want 1/1", successes, collisions)
	}
	list, err := docs.List(ctx, "u-doc-agg", nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("winner list=%+v err=%v", list, err)
	}
	winner := list[0]
	if winner.Body != "[["+winner.Title+"]]" || len(winner.Tags) != 1 || winner.Tags[0] != winner.Title {
		t.Fatalf("winner aggregate diverged: %+v", winner)
	}
	backlinks, err := docs.Backlinks(ctx, "u-doc-agg", winner.Title)
	if err != nil || !containsDocumentID(backlinks, winner.ID) {
		t.Fatalf("winner link missing: doc=%+v backlinks=%+v err=%v", winner, backlinks, err)
	}
}

func containsDocumentID(docs []domain.Document, id string) bool {
	for _, doc := range docs {
		if doc.ID == id {
			return true
		}
	}
	return false
}
