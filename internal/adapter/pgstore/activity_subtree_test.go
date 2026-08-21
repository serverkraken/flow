package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

// TestActivityStore_SubtreeQueries covers the register entry point's two
// subtree-scoped reads. They exist so the "last 10" and the agent tally are
// TRUE rather than approximate: filtering an owner-wide page in the adapter
// silently drops a register's rows as soon as a louder neighbour fills the
// page.
func TestActivityStore_SubtreeQueries(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewActivityStore(pool)
	owner := "owner-subtree"
	base := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	node := func(s string) *string { return &s }

	seed := []domain.ActivityEntry{
		{ID: "s1", OwnerID: owner, ActorKind: "human", ActorRef: "msoent", Kind: "session.started", NodeRef: node("mine-a"), At: base},
		{ID: "s2", OwnerID: owner, ActorKind: "agent", ActorRef: "claude-code", Kind: "document.updated", NodeRef: node("mine-b"), At: base.Add(time.Minute)},
		{ID: "s3", OwnerID: owner, ActorKind: "agent", ActorRef: "claude-code", Kind: "document.updated", NodeRef: node("mine-a"), At: base.Add(2 * time.Minute)},
		{ID: "s4", OwnerID: owner, ActorKind: "agent", ActorRef: "codex", Kind: "document.updated", NodeRef: node("fremd"), At: base.Add(3 * time.Minute)},
		// A row without a node at all (free note): belongs to no register.
		{ID: "s5", OwnerID: owner, ActorKind: "human", ActorRef: "msoent", Kind: "document.created", At: base.Add(4 * time.Minute)},
		// Another owner, same node ids — must never appear.
		{ID: "s6", OwnerID: "owner-other", ActorKind: "agent", ActorRef: "eindringling", Kind: "document.updated", NodeRef: node("mine-a"), At: base.Add(5 * time.Minute)},
	}
	for _, e := range seed {
		if err := st.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	mine := []string{"mine-a", "mine-b"}

	items, total, err := st.ListPageForNodes(ctx, owner, mine, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (the subtree's rows, not the owner's)", total)
	}
	if len(items) != 3 || items[0].ID != "s3" {
		t.Errorf("items = %+v, want s3, s2, s1 newest first", items)
	}
	for _, it := range items {
		if it.ID == "s4" || it.ID == "s5" || it.ID == "s6" {
			t.Errorf("row %s must not be in the subtree page", it.ID)
		}
	}

	// The page really pages.
	page2, _, err := st.ListPageForNodes(ctx, owner, mine, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].ID != "s1" {
		t.Errorf("second page = %+v, want just s1", page2)
	}

	// No nodes → nothing, and no attempt at a broader read.
	if items, total, err := st.ListPageForNodes(ctx, owner, nil, 10, 0); err != nil || len(items) != 0 || total != 0 {
		t.Errorf("empty node set = %+v total=%d err=%v, want empty", items, total, err)
	}

	// Distinct AGENTS in the subtree since a cutoff — humans do not count.
	n, err := st.DistinctAgentsSince(ctx, owner, mine, base)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("agents = %d, want 1 (claude-code twice is still one agent; codex is elsewhere)", n)
	}
	if n, err := st.DistinctAgentsSince(ctx, owner, mine, base.Add(time.Hour)); err != nil || n != 0 {
		t.Errorf("agents after the cutoff = %d err=%v, want 0", n, err)
	}
	if n, err := st.DistinctAgentsSince(ctx, "owner-other", mine, base); err != nil || n != 1 {
		t.Errorf("the other owner counts only their own: %d err=%v", n, err)
	}
}
