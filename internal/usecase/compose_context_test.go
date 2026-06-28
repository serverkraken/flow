package usecase_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func node(id, name string, k domain.NodeKind) domain.Node {
	return domain.Node{ID: id, Name: name, Slug: name, Kind: k}
}
func doc(id string, node *string, typ domain.DocumentType, path string, pinned bool, updated time.Time, body string) domain.Document {
	return domain.Document{ID: id, NodeID: node, Type: typ, Path: path, Pinned: pinned, UpdatedAt: updated, Body: body}
}

func TestCompose_TiersAndActiveContext(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("i1", &leaf, domain.DocInstruction, "claude", false, t0, "rules"),
		doc("i0", nil, domain.DocInstruction, "claude", false, t0, "global rules"),
		doc("ac", &leaf, domain.DocMemory, usecase.ActiveContextPath, false, t0, "where I was"),
		doc("ml", &leaf, domain.DocMemory, "m-leaf", false, t0, "leaf mem"),
		doc("me", &eng, domain.DocMemory, "m-eng", false, t0, "eng mem"),
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)

	if len(got.Instructions) != 2 {
		t.Errorf("want 2 instructions (chain+global), got %d", len(got.Instructions))
	}
	if got.ActiveContext == nil || got.ActiveContext.ID != "ac" {
		t.Fatalf("activeContext not extracted: %+v", got.ActiveContext)
	}
	if len(got.Memories["leaf"]) != 1 || got.Memories["leaf"][0].ID != "ml" {
		t.Errorf("leaf memory tier wrong: %+v", got.Memories["leaf"])
	}
	if len(got.Memories["engagement"]) != 1 || got.Memories["engagement"][0].ID != "me" {
		t.Errorf("engagement memory tier wrong: %+v", got.Memories["engagement"])
	}
	// activeContext must NOT also appear in Memories["leaf"].
	for _, m := range got.Memories["leaf"] {
		if m.ID == "ac" {
			t.Errorf("activeContext double-counted in leaf memories")
		}
	}
}

func TestCompose_BudgetDropsRelevanceByRank(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // n bytes → EstTokens = ceil(n/4)
	docs := []domain.Document{
		// three engagement memories; each EstTokens=100 (400 bytes). cap=250 → only 2 fit.
		// Inserted intentionally NOT pre-sorted (pinnedOld LAST) so the sort is load-bearing:
		// without the sort the result would be [freshUnpinned, olderUnpinned] and the assertion fails.
		doc("freshUnpinned", &eng, domain.DocMemory, "b", false, mid, body(400)),
		doc("olderUnpinned", &eng, domain.DocMemory, "c", false, old, body(400)),
		doc("pinnedOld", &eng, domain.DocMemory, "a", true, old, body(400)),
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 250)
	kept := got.Memories["engagement"]
	if len(kept) != 2 {
		t.Fatalf("cap=250 with 3×100-token items should keep 2, got %d", len(kept))
	}
	// rank: pinned first, then newer. So pinnedOld + freshUnpinned kept; olderUnpinned dropped.
	if kept[0].ID != "pinnedOld" || kept[1].ID != "freshUnpinned" {
		t.Errorf("rank wrong: %s,%s", kept[0].ID, kept[1].ID)
	}
	if got.Budget.Dropped.Engagement != 1 {
		t.Errorf("want 1 dropped engagement, got %d", got.Budget.Dropped.Engagement)
	}
}

func TestCompose_GlobalGatedByTag(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("gAllowed", nil, domain.DocMemory, "g1", false, t0, "x"),
		doc("gBlocked", nil, domain.DocMemory, "g2", false, t0, "y"),
	}
	got := usecase.Compose(chain, docs, map[string]bool{"gAllowed": true}, 100000)
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gAllowed" {
		t.Fatalf("only tag-allowed global memory should pass: %+v", got.Memories["global"])
	}
}

func TestCompose_UnresolvedNotHandledHere(t *testing.T) {
	t.Parallel()
	// Compose with an empty chain treats everything as global candidates only.
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{doc("g", nil, domain.DocMemory, "g", false, t0, "x")}
	got := usecase.Compose(nil, docs, map[string]bool{"g": true}, 100000)
	if len(got.Memories["global"]) != 1 {
		t.Fatalf("empty chain should still surface gated global memories")
	}
	if !got.Resolution.Unresolved {
		t.Errorf("empty chain should set Resolution.Unresolved")
	}
}

func TestCompose_SingleEngagementChainLeafTier(t *testing.T) {
	t.Parallel()
	e := "E"
	chain := []domain.Node{node(e, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{doc("m", &e, domain.DocMemory, "m", false, t0, "x")}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)
	if len(got.Memories["leaf"]) != 1 || got.Memories["leaf"][0].ID != "m" {
		t.Fatalf("single-engagement-chain memory should be leaf/always-tier, got %+v", got.Memories)
	}
	if len(got.Memories["engagement"]) != 0 {
		t.Errorf("no engagement-tier when leaf==root: %+v", got.Memories["engagement"])
	}
}
