package pgstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func snapshotHash(t *testing.T, st *pgstore.DocumentStore, id string) string {
	t.Helper()
	stale, err := st.StaleDocuments(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, sd := range stale {
		if sd.Doc.ID == id {
			return sd.SnapshotHash
		}
	}
	t.Fatalf("document %s is not stale", id)
	return ""
}

func TestReplaceChunks_RejectsStaleAndDeletedSnapshots(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-cas", "sub-cas", "cas", "cas@x.de", "CAS")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	doc := domain.Document{ID: "d-cas", OwnerID: u.ID, Type: domain.DocFree, Path: "d-cas", Title: "old", Body: "body", CreatedAt: now, UpdatedAt: now}
	if _, err := st.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	oldHash := snapshotHash(t, st, doc.ID)
	doc.Title = "new"
	doc.UpdatedAt = now.Add(time.Second)
	if _, err := st.Update(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceChunks(ctx, doc.ID, doc.OwnerID, oldHash, []string{"old chunk"}, [][]float32{vec768(0)}); !errors.Is(err, ports.ErrEmbedStaleSnapshot) {
		t.Fatalf("stale snapshot: want ErrEmbedStaleSnapshot, got %v", err)
	}
	if status, err := st.EmbedStatus(ctx, doc.OwnerID, doc.ID); err != nil || status.State != domain.EmbedPending {
		t.Fatalf("new content must remain pending, status=%#v err=%v", status, err)
	}
	newHash := snapshotHash(t, st, doc.ID)
	if err := st.Delete(ctx, doc.OwnerID, doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceChunks(ctx, doc.ID, doc.OwnerID, newHash, nil, nil); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("deleted snapshot: want ErrDocumentNotFound, got %v", err)
	}
}

func TestEmbeddingSnapshot_DistinguishesTitleBodyBoundary(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-boundary", "sub-boundary", "boundary", "boundary@example.test", "Boundary")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	doc := domain.Document{
		ID: "d-boundary", OwnerID: u.ID, Type: domain.DocFree, Path: "boundary",
		Title: "ab", Body: "c", CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	oldHash := snapshotHash(t, st, doc.ID)
	if err := st.ReplaceChunks(ctx, doc.ID, doc.OwnerID, oldHash, []string{"ab\n\nc"}, [][]float32{vec768(0)}); err != nil {
		t.Fatal(err)
	}

	doc.Title, doc.Body, doc.UpdatedAt = "a", "bc", now.Add(time.Second)
	if _, err := st.Update(ctx, doc); err != nil {
		t.Fatal(err)
	}
	status, err := st.EmbedStatus(ctx, doc.OwnerID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != domain.EmbedPending {
		t.Fatalf("boundary-changing edit must invalidate chunks, status=%s", status.State)
	}
	newHash := snapshotHash(t, st, doc.ID)
	if newHash == oldHash {
		t.Fatalf("snapshot hash did not distinguish title/body boundary: %q", newHash)
	}
}

func TestDocumentContentWrites_ClearObsoleteEmbedFailures(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-failure-reset", "sub-failure-reset", "failure-reset", "failure-reset@example.test", "Failure Reset")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	for _, id := range []string{"d-aggregate-reset", "d-direct-reset", "d-upsert-reset", "d-direct-upsert-reset"} {
		doc := domain.Document{
			ID: id, OwnerID: u.ID, Type: domain.DocFree, Path: id,
			Title: "Old", Body: "old body", CreatedAt: now, UpdatedAt: now,
		}
		if _, err := st.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
		if err := st.RecordEmbedFailure(ctx, id, u.ID, snapshotHash(t, st, id), 5, now, true, "old content failed"); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := st.UpdateDocumentAggregate(ctx, u.ID, "d-aggregate-reset", func(doc domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		doc.Title, doc.Body, doc.UpdatedAt = "New aggregate", "new body", now.Add(time.Second)
		return doc, ports.DocumentAggregateChanges{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	direct, err := st.Get(ctx, u.ID, "d-direct-reset")
	if err != nil {
		t.Fatal(err)
	}
	direct.Title, direct.Body, direct.UpdatedAt = "New direct", "new body", now.Add(time.Second)
	if _, err := st.Update(ctx, direct); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
		OwnerID: u.ID, Type: domain.DocFree, Path: "d-upsert-reset",
		Title: "New upsert", Body: "new body",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpsertByPath(ctx, u.ID, nil, domain.DocFree, "d-direct-upsert-reset", "New direct upsert", "new body", false, false, "human", ""); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"d-aggregate-reset", "d-direct-reset", "d-upsert-reset", "d-direct-upsert-reset"} {
		status, err := st.EmbedStatus(ctx, u.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != domain.EmbedPending || status.Attempts != 0 || status.LastError != "" {
			t.Fatalf("%s retained obsolete failure: %#v", id, status)
		}
		_ = snapshotHash(t, st, id)
	}

	rollbackIDs := []string{"d-aggregate-rollback", "d-direct-rollback", "d-upsert-rollback", "d-direct-upsert-rollback"}
	for _, id := range rollbackIDs {
		doc := domain.Document{
			ID: id, OwnerID: u.ID, Type: domain.DocFree, Path: id,
			Title: "Old", Body: "old body", CreatedAt: now, UpdatedAt: now,
		}
		if _, err := st.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
		if err := st.RecordEmbedFailure(ctx, id, u.ID, snapshotHash(t, st, id), 5, now, true, "must survive rollback"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION reject_test_embed_failure_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.document_id LIKE 'd-%-rollback' THEN
    RAISE EXCEPTION 'injected embed failure delete error';
  END IF;
  RETURN OLD;
END $$;
CREATE TRIGGER reject_test_embed_failure_delete
BEFORE DELETE ON document_embed_failures
FOR EACH ROW EXECUTE FUNCTION reject_test_embed_failure_delete()`); err != nil {
		t.Fatal(err)
	}

	if _, err := st.UpdateDocumentAggregate(ctx, u.ID, "d-aggregate-rollback", func(doc domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		doc.Title, doc.Body, doc.UpdatedAt = "New aggregate", "new body", now.Add(time.Second)
		return doc, ports.DocumentAggregateChanges{}, nil
	}); err == nil {
		t.Fatal("aggregate update succeeded despite injected failure clear error")
	}
	direct, err = st.Get(ctx, u.ID, "d-direct-rollback")
	if err != nil {
		t.Fatal(err)
	}
	direct.Title, direct.Body, direct.UpdatedAt = "New direct", "new body", now.Add(time.Second)
	if _, err := st.Update(ctx, direct); err == nil {
		t.Fatal("direct update succeeded despite injected failure clear error")
	}
	if _, err := st.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
		OwnerID: u.ID, Type: domain.DocFree, Path: "d-upsert-rollback",
		Title: "New upsert", Body: "new body",
	}); err == nil {
		t.Fatal("aggregate upsert succeeded despite injected failure clear error")
	}
	if _, _, err := st.UpsertByPath(ctx, u.ID, nil, domain.DocFree, "d-direct-upsert-rollback", "New direct upsert", "new body", false, false, "human", ""); err == nil {
		t.Fatal("direct upsert succeeded despite injected failure clear error")
	}

	for _, id := range rollbackIDs {
		doc, err := st.Get(ctx, u.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Title != "Old" || doc.Body != "old body" {
			t.Fatalf("%s content escaped rollback: title=%q body=%q", id, doc.Title, doc.Body)
		}
		status, err := st.EmbedStatus(ctx, u.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != domain.EmbedFailed || status.Attempts != 5 || status.LastError != "must survive rollback" {
			t.Fatalf("%s failure state escaped rollback: %#v", id, status)
		}
	}
}

func TestEmbeddingSnapshot_ConcurrentContentMutationRejectsObsoleteResult(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-embed-race", "sub-embed-race", "embed-race", "embed-race@example.test", "Embed Race")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	vector := make([]float32, 768)
	vector[0] = 1

	for _, result := range []string{"failure", "chunks"} {
		result := result
		t.Run(result, func(t *testing.T) {
			for i := 0; i < 8; i++ {
				id := fmt.Sprintf("d-embed-race-%s-%d", result, i)
				doc := domain.Document{
					ID: id, OwnerID: u.ID, Type: domain.DocFree, Path: id,
					Title: "Old", Body: "old body", CreatedAt: now, UpdatedAt: now,
				}
				if _, err := st.Create(ctx, doc); err != nil {
					t.Fatal(err)
				}
				oldHash := snapshotHash(t, st, id)
				start := make(chan struct{})
				updateErr := make(chan error, 1)
				resultErr := make(chan error, 1)
				go func() {
					<-start
					_, err := st.UpdateDocumentAggregate(ctx, u.ID, id, func(current domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
						current.Title = "New"
						current.Body = "new body"
						current.UpdatedAt = now.Add(time.Duration(i+1) * time.Second)
						return current, ports.DocumentAggregateChanges{}, nil
					})
					updateErr <- err
				}()
				go func() {
					<-start
					if result == "failure" {
						resultErr <- st.RecordEmbedFailure(ctx, id, u.ID, oldHash, 5, now, true, "obsolete failure")
						return
					}
					resultErr <- st.ReplaceChunks(ctx, id, u.ID, oldHash, []string{"obsolete chunk"}, [][]float32{vector})
				}()
				close(start)

				if err := <-updateErr; err != nil {
					t.Fatalf("content update: %v", err)
				}
				if err := <-resultErr; err != nil && !errors.Is(err, ports.ErrEmbedStaleSnapshot) {
					t.Fatalf("obsolete %s result: %v", result, err)
				}
				status, err := st.EmbedStatus(ctx, u.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if status.State != domain.EmbedPending || status.Attempts != 0 || status.LastError != "" {
					t.Fatalf("obsolete %s won race: %#v", result, status)
				}
				newHash := snapshotHash(t, st, id)
				if newHash == oldHash {
					t.Fatal("content mutation retained obsolete snapshot hash")
				}
			}
		})
	}
}

func TestEmbedFailures_RecordExcludeClearStatus(t *testing.T) {
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
	u, _ := domain.NewUser("u-emb", "sub-emb", "emb", "emb@x.de", "Emb")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string) domain.Document {
		return domain.Document{ID: id, OwnerID: "u-emb", Type: domain.DocFree, Path: id, Title: id, Body: "b", CreatedAt: now, UpdatedAt: now}
	}
	if _, err := st.Create(ctx, mk("d-dead")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, mk("d-future")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, mk("d-due")); err != nil {
		t.Fatal(err)
	}

	// dead-lettered → excluded; pending status reads as "pending" until recorded
	if s, err := st.EmbedStatus(ctx, "u-emb", "d-dead"); err != nil || s.State != domain.EmbedPending {
		t.Fatalf("fresh doc want pending, got %v err=%v", s.State, err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-dead", "u-emb", snapshotHash(t, st, "d-dead"), 5, now, true, "boom"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-future", "u-emb", snapshotHash(t, st, "d-future"), 2, now.Add(time.Hour), false, "later"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-due", "u-emb", snapshotHash(t, st, "d-due"), 1, now.Add(-time.Hour), false, "due"); err != nil {
		t.Fatal(err)
	}

	stale, err := st.StaleDocuments(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, sd := range stale {
		got[sd.Doc.ID] = sd.Attempts
	}
	if _, ok := got["d-dead"]; ok {
		t.Fatalf("dead doc must be excluded: %v", got)
	}
	if _, ok := got["d-future"]; ok {
		t.Fatalf("backing-off doc must be excluded: %v", got)
	}
	if got["d-due"] != 1 {
		t.Fatalf("due doc must be present with attempts=1, got %v", got)
	}

	// status reflects dead + retrying
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-dead"); s.State != domain.EmbedFailed || s.LastError != "boom" {
		t.Fatalf("dead status: %#v", s)
	}
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-future"); s.State != domain.EmbedRetrying || s.NextRetry == nil {
		t.Fatalf("retrying status: %#v", s)
	}

	// clear restores eligibility
	if err := st.ClearEmbedFailure(ctx, "d-dead", "u-emb"); err != nil {
		t.Fatal(err)
	}
	stale, _ = st.StaleDocuments(ctx, 100)
	found := false
	for _, sd := range stale {
		if sd.Doc.ID == "d-dead" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cleared doc must be stale again")
	}

	// success clears the row + flips status to ok
	if err := st.ReplaceChunks(ctx, "d-due", "u-emb", snapshotHash(t, st, "d-due"), []string{"c"}, [][]float32{vec768(0)}); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-due"); s.State != domain.EmbedOK {
		t.Fatalf("embedded doc want ok, got %v", s.State)
	}
}
