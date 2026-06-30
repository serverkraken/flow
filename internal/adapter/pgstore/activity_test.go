package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func ptr(s string) *string { return &s }

func TestActivityStore(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// We need a user to satisfy FK on activity.owner_id — but activity table
	// has no FK to users (it's a plain TEXT column), so no user seeding needed.

	store := pgstore.NewActivityStore(pool)
	ownerA := "owner-a"
	ownerB := "owner-b"

	base := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	// Seed 3 entries for owner A (mixed kinds + actor_refs).
	docLabel := "My Doc"
	entries := []domain.ActivityEntry{
		{
			ID:        "act-a1",
			OwnerID:   ownerA,
			ActorKind: "human",
			ActorRef:  "msoent",
			Kind:      "document.updated",
			TargetRef: ptr("doc-1"),
			Label:     &docLabel,
			At:        base.Add(2 * time.Hour),
		},
		{
			ID:        "act-a2",
			OwnerID:   ownerA,
			ActorKind: "agent",
			ActorRef:  "claude-code",
			Kind:      "session.started",
			At:        base.Add(1 * time.Hour),
		},
		{
			ID:        "act-a3",
			OwnerID:   ownerA,
			ActorKind: "human",
			ActorRef:  "msoent",
			Kind:      "document.created",
			TargetRef: ptr("doc-2"),
			At:        base,
		},
	}
	for _, e := range entries {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("Append %q: %v", e.ID, err)
		}
	}

	// Seed 1 entry for owner B — must never leak to owner A queries.
	entryB := domain.ActivityEntry{
		ID:        "act-b1",
		OwnerID:   ownerB,
		ActorKind: "human",
		ActorRef:  "other",
		Kind:      "document.updated",
		At:        base,
	}
	if err := store.Append(ctx, entryB); err != nil {
		t.Fatalf("Append owner B: %v", err)
	}

	t.Run("all entries for owner A, newest-first", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, nil, nil, 50, 0)
		if err != nil {
			t.Fatalf("ListPage: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		// newest-first: act-a1 (base+2h) > act-a2 (base+1h) > act-a3 (base)
		if items[0].ID != "act-a1" || items[1].ID != "act-a2" || items[2].ID != "act-a3" {
			t.Fatalf("wrong order: %v %v %v", items[0].ID, items[1].ID, items[2].ID)
		}
		// check label is round-tripped
		if items[0].Label == nil || *items[0].Label != docLabel {
			t.Fatalf("label not round-tripped: %v", items[0].Label)
		}
	})

	t.Run("class prefix filter document.* returns only document events", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, []string{"document"}, nil, 50, 0)
		if err != nil {
			t.Fatalf("ListPage document class: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
		if len(items) != 2 {
			t.Fatalf("len = %d, want 2", len(items))
		}
		for _, it := range items {
			if len(it.Kind) < 9 || it.Kind[:8] != "document" {
				t.Fatalf("unexpected kind %q in document filter", it.Kind)
			}
		}
	})

	t.Run("actorRef filter returns only claude-code entries", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, nil, ptr("claude-code"), 50, 0)
		if err != nil {
			t.Fatalf("ListPage actorRef: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1", total)
		}
		if len(items) != 1 || items[0].ID != "act-a2" {
			t.Fatalf("wrong items: %+v", items)
		}
	})

	t.Run("pagination: limit 2 offset 0 returns 2 items, total still 3", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, nil, nil, 2, 0)
		if err != nil {
			t.Fatalf("ListPage page 1: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(items) != 2 {
			t.Fatalf("len = %d, want 2", len(items))
		}
	})

	t.Run("pagination: limit 2 offset 2 returns 1 item, total still 3", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, nil, nil, 2, 2)
		if err != nil {
			t.Fatalf("ListPage page 2: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(items) != 1 {
			t.Fatalf("len = %d, want 1", len(items))
		}
	})

	t.Run("owner isolation: owner B row does not appear for owner A", func(t *testing.T) {
		items, total, err := store.ListPage(ctx, ownerA, nil, nil, 50, 0)
		if err != nil {
			t.Fatalf("ListPage owner isolation: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3 (owner B leaked?)", total)
		}
		for _, it := range items {
			if it.ID == "act-b1" {
				t.Fatal("owner B entry leaked into owner A results")
			}
		}
	})

	t.Run("class and actor combined filter", func(t *testing.T) {
		// "document" class + "msoent" actor → 2 matches (act-a1 document.updated, act-a3 document.created)
		items, total, err := store.ListPage(ctx, ownerA, []string{"document"}, ptr("msoent"), 50, 0)
		if err != nil {
			t.Fatalf("ListPage combined: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
		if len(items) != 2 {
			t.Fatalf("len = %d, want 2", len(items))
		}
	})
}
