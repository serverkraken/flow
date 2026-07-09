package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestSessionStore_LastBookedByNode(t *testing.T) {
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
	for _, id := range []string{"u1", "u2"} {
		u, _ := domain.NewUser(id, "sub-"+id, id, id+"@x.de", id)
		if _, err := users.UpsertBySub(ctx, u); err != nil {
			t.Fatalf("seed user %s: %v", id, err)
		}
	}
	nodes := pgstore.NewNodeStore(pool)
	seed := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	for _, slug := range []string{"n1", "n2"} {
		n, _ := domain.NewNode(slug, "u1", slug, slug, seed)
		n.Kind = domain.KindEngagement
		if _, err := nodes.Create(ctx, n); err != nil {
			t.Fatalf("seed node %s: %v", slug, err)
		}
	}

	store := pgstore.NewSessionStore(pool)
	n1, n2 := "n1", "n2"
	mk := func(id, owner string, node *string, start time.Time, stopped bool) {
		ws := domain.WorkSession{ID: id, OwnerID: owner, NodeID: node, Start: start, CreatedAt: start}
		if stopped {
			s := start.Add(time.Hour)
			ws.Stop = &s
		}
		if _, err := store.Create(ctx, ws); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mk("a", "u1", &n1, base, true)                   // n1 older
	mk("b", "u1", &n1, base.AddDate(0, 0, 5), true)  // n1 newest → this start wins
	mk("c", "u1", &n2, base.AddDate(0, 0, 2), true)  // n2
	mk("d", "u1", &n1, base.AddDate(0, 0, 9), false) // running → ignored
	mk("e", "u1", nil, base.AddDate(0, 0, 9), true)  // unbooked → ignored
	mk("f", "u2", &n1, base.AddDate(0, 0, 9), true)  // other owner → ignored for u1

	got, err := store.LastBookedByNode(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 nodes, got %d (%v)", len(got), got)
	}
	if !got[n1].Equal(base.AddDate(0, 0, 5)) {
		t.Errorf("n1 last-booked = %v, want %v (newest stopped booked start)", got[n1], base.AddDate(0, 0, 5))
	}
	if !got[n2].Equal(base.AddDate(0, 0, 2)) {
		t.Errorf("n2 last-booked = %v", got[n2])
	}
	// Owner-scope: u2 sees only its own.
	if g2, _ := store.LastBookedByNode(ctx, "u2"); len(g2) != 1 || !g2[n1].Equal(base.AddDate(0, 0, 9)) {
		t.Errorf("owner-scope leak: %v", g2)
	}
}
