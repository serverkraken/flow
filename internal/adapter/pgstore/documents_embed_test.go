package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

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
	st := pgstore.NewDocumentStore(pool)
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
	if err := st.RecordEmbedFailure(ctx, "d-dead", "u-emb", 5, now, true, "boom"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-future", "u-emb", 2, now.Add(time.Hour), false, "later"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEmbedFailure(ctx, "d-due", "u-emb", 1, now.Add(-time.Hour), false, "due"); err != nil {
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
	if err := st.ReplaceChunks(ctx, "d-due", "u-emb", []string{"c"}, [][]float32{vec768(0)}); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.EmbedStatus(ctx, "u-emb", "d-due"); s.State != domain.EmbedOK {
		t.Fatalf("embedded doc want ok, got %v", s.State)
	}
}
