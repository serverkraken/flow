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

func TestNodeHighlightStore_OrdersAndScopes(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-hl", "sub-hl", "hl", "hl@x.de", "HL")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	nodes := pgstore.NewNodeStore(pool)
	mkNode := func(id string) {
		n, _ := domain.NewNode(id, u.ID, id, id, now)
		n.Kind = domain.KindEngagement
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	mkNode("hl-eng-a")
	mkNode("hl-eng-b")
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	mkDoc := func(id string) {
		if _, err := docs.Create(ctx, domain.Document{
			ID: id, OwnerID: u.ID, Type: domain.DocDaily, Path: "daily/" + id,
			Title: "Tagesnotiz " + id, Body: "Inhalt", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mkDoc("hl-doc-1")
	mkDoc("hl-doc-2")

	st := pgstore.NewNodeHighlightStore(pool)
	mk := func(id, docID, nodeID, quote string, at time.Time) domain.NodeHighlight {
		h, err := st.Create(ctx, domain.NodeHighlight{
			ID: id, OwnerID: u.ID, DocumentID: docID, NodeID: nodeID, Quote: quote, CreatedAt: at,
		})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	old := mk("hl-1", "hl-doc-1", "hl-eng-a", "die ältere Stelle", now.Add(-48*time.Hour))
	mid := mk("hl-2", "hl-doc-1", "hl-eng-b", "die mittlere Stelle", now.Add(-24*time.Hour))
	newest := mk("hl-3", "hl-doc-2", "hl-eng-a", "die neueste Stelle", now)

	// ListForDocument is reading order: oldest first, and it never leaks the
	// other document's marks.
	inDoc, err := st.ListForDocument(ctx, u.ID, "hl-doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(inDoc) != 2 || inDoc[0].ID != old.ID || inDoc[1].ID != mid.ID {
		t.Errorf("ListForDocument = %+v, want [%s %s] in reading order", inDoc, old.ID, mid.ID)
	}

	// ListRecent is newest first and honours the cap.
	recent, err := st.ListRecent(ctx, u.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != newest.ID || recent[1].ID != mid.ID {
		t.Errorf("ListRecent(2) = %+v, want [%s %s] newest first", recent, newest.ID, mid.ID)
	}

	// ListSince cuts on created_at.
	since, err := st.ListSince(ctx, u.ID, now.Add(-36*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(since) != 2 {
		t.Errorf("ListSince = %+v, want the two newer marks", since)
	}

	// Owner scope: a foreign owner sees nothing and deletes nothing.
	if l, err := st.ListRecent(ctx, "u-fremd", 10); err != nil || len(l) != 0 {
		t.Errorf("foreign ListRecent = %+v err=%v, want empty", l, err)
	}
	if l, err := st.ListForDocument(ctx, "u-fremd", "hl-doc-1"); err != nil || len(l) != 0 {
		t.Errorf("foreign ListForDocument = %+v err=%v, want empty", l, err)
	}
	if err := st.Delete(ctx, "u-fremd", newest.ID); !errors.Is(err, ports.ErrNodeHighlightNotFound) {
		t.Errorf("foreign delete: want ErrNodeHighlightNotFound, got %v", err)
	}

	if err := st.Delete(ctx, u.ID, newest.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.Delete(ctx, u.ID, newest.ID); !errors.Is(err, ports.ErrNodeHighlightNotFound) {
		t.Errorf("second delete: want ErrNodeHighlightNotFound, got %v", err)
	}
}

// TestNodeHighlightStore_ListRecentForNodes covers the register entry point's
// "Woran zuletzt gearbeitet": newest marks from THIS register's subtree, not
// the owner's newest filtered afterwards — a busy neighbour would otherwise
// push a quiet register's marks out of the window entirely.
func TestNodeHighlightStore_ListRecentForNodes(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-hlsub", "sub-hlsub", "hlsub", "hlsub@x.de", "HLSub")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	nodes := pgstore.NewNodeStore(pool)
	for _, id := range []string{"sub-mine", "sub-loud"} {
		n, _ := domain.NewNode(id, u.ID, id, id, now)
		n.Kind = domain.KindEngagement
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	if _, err := docs.Create(ctx, domain.Document{
		ID: "hlsub-doc", OwnerID: u.ID, Type: domain.DocDaily, Path: "daily/hlsub",
		Title: "Notiz", Body: "Text", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeHighlightStore(pool)
	mk := func(id, nodeID string, at time.Time) {
		if _, err := st.Create(ctx, domain.NodeHighlight{
			ID: id, OwnerID: u.ID, DocumentID: "hlsub-doc", NodeID: nodeID,
			Quote: "Stelle " + id, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("q-mine-old", "sub-mine", now.Add(-72*time.Hour))
	mk("q-mine-new", "sub-mine", now.Add(-48*time.Hour))
	// The loud neighbour marks three fresher passages.
	mk("q-loud-1", "sub-loud", now.Add(-3*time.Hour))
	mk("q-loud-2", "sub-loud", now.Add(-2*time.Hour))
	mk("q-loud-3", "sub-loud", now.Add(-1*time.Hour))

	got, err := st.ListRecentForNodes(ctx, u.ID, []string{"sub-mine"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "q-mine-new" || got[1].ID != "q-mine-old" {
		t.Errorf("got %+v, want the quiet register's two marks newest first", got)
	}
	if l, err := st.ListRecentForNodes(ctx, u.ID, []string{"sub-mine"}, 1); err != nil || len(l) != 1 {
		t.Errorf("limit ignored: %+v err=%v", l, err)
	}
	if l, err := st.ListRecentForNodes(ctx, u.ID, nil, 5); err != nil || len(l) != 0 {
		t.Errorf("empty node set = %+v err=%v, want empty", l, err)
	}
	if l, err := st.ListRecentForNodes(ctx, "u-fremd", []string{"sub-mine"}, 5); err != nil || len(l) != 0 {
		t.Errorf("foreign owner = %+v err=%v, want empty", l, err)
	}
}
