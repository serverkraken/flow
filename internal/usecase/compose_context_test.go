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
		doc("ac", &leaf, domain.DocActiveContext, usecase.ActiveContextPath, false, t0, "where I was"),
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

func TestCompose_ActiveContextByType(t *testing.T) {
	t.Parallel()
	leaf := domain.Node{ID: "n1", Kind: domain.KindRepo, Name: "flow"}
	chain := []domain.Node{leaf}
	docs := []domain.Document{
		{ID: "ac", NodeID: &leaf.ID, Type: domain.DocActiveContext, Path: "active-context", Body: "where I was"},
		{ID: "m1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: "some-note", Body: "a memory"},
	}
	out := usecase.Compose(chain, docs, map[string]bool{}, 6000)
	if out.ActiveContext == nil || out.ActiveContext.ID != "ac" {
		t.Fatalf("activeContext not picked up by type: %+v", out.ActiveContext)
	}
	if len(out.Memories["leaf"]) != 1 || out.Memories["leaf"][0].ID != "m1" {
		t.Fatalf("leaf memory misrouted: %+v", out.Memories["leaf"])
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

func TestCompose_PinnedGlobalBypassesGate(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		doc("gPinned", nil, domain.DocMemory, "g1", true, t0, "always"),  // pinned, NOT in globalAllowed
		doc("gPlain", nil, domain.DocMemory, "g2", false, t0, "topical"), // unpinned, NOT in globalAllowed
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000) // empty globalAllowed
	if len(got.Memories["global"]) != 1 || got.Memories["global"][0].ID != "gPinned" {
		t.Fatalf("pinned global must bypass D7; unpinned stays gated: %+v", got.Memories["global"])
	}
}

func TestCompose_TierRankFillOrder(t *testing.T) {
	t.Parallel()
	leaf, vor, eng := "L", "V", "E"
	chain := []domain.Node{
		node(leaf, "flow", domain.KindRepo),
		node(vor, "Vorhaben", domain.KindVorhaben),
		node(eng, "Privat", domain.KindEngagement),
	}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // EstTokens = ceil(n/4)
	docs := []domain.Document{
		doc("le", &leaf, domain.DocMemory, "l", false, t0, body(400)),
		doc("vo", &vor, domain.DocMemory, "v", false, t0, body(400)),
		doc("en", &eng, domain.DocMemory, "e", false, t0, body(400)),
		doc("gl", nil, domain.DocMemory, "g", false, t0, body(400)),
	}
	// 4×100-tok items, cap=250 → two highest tiers (global, engagement) fit; vorhaben+leaf drop.
	got := usecase.Compose(chain, docs, map[string]bool{"gl": true}, 250)
	if len(got.Memories["global"]) != 1 || len(got.Memories["engagement"]) != 1 {
		t.Fatalf("global+engagement should survive tight cap: %+v", got.Memories)
	}
	if len(got.Memories["vorhaben"]) != 0 || len(got.Memories["leaf"]) != 0 {
		t.Fatalf("vorhaben+leaf should drop: %+v", got.Memories)
	}
	if got.Budget.Dropped.Vorhaben != 1 || got.Budget.Dropped.Leaf != 1 {
		t.Errorf("want vorhaben=1 leaf=1 dropped, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_PinnedBeatsTier(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("leafPinned", &leaf, domain.DocMemory, "l", true, t0, body(400)), // pinned leaf (rank 3)
		doc("globalPlain", nil, domain.DocMemory, "g", false, t0, body(400)), // unpinned global (rank 0)
	}
	// cap=100 → exactly one 100-tok item fits; pinned beats tier → leafPinned wins.
	got := usecase.Compose(chain, docs, map[string]bool{"globalPlain": true}, 100)
	if len(got.Memories["leaf"]) != 1 || got.Memories["leaf"][0].ID != "leafPinned" {
		t.Fatalf("pinned leaf must win over unpinned higher-tier global: %+v", got.Memories)
	}
	if got.Budget.Dropped.Global != 1 {
		t.Errorf("unpinned global should drop, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_DroppedPinnedSignaled(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("p1", &leaf, domain.DocMemory, "a", true, t0, body(400)), // pinned 100 tok
		doc("p2", &leaf, domain.DocMemory, "b", true, t0, body(400)), // pinned 100 tok
	}
	// cap=100 → one pinned fits, one drops → Dropped.Pinned=1 AND Dropped.Leaf=1.
	got := usecase.Compose(chain, docs, map[string]bool{}, 100)
	if got.Budget.Dropped.Pinned != 1 {
		t.Errorf("a dropped pin must set Dropped.Pinned, got %+v", got.Budget.Dropped)
	}
	if got.Budget.Dropped.Leaf != 1 {
		t.Errorf("the dropped pin is a leaf → Dropped.Leaf must also count it, got %+v", got.Budget.Dropped)
	}
}

func TestCompose_FloorExceedsCapKeepsAlways(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("instr", &leaf, domain.DocInstruction, "claude", false, t0, body(800)),                 // 200 tok
		doc("ac", &leaf, domain.DocActiveContext, usecase.ActiveContextPath, false, t0, body(400)), // 100 tok
		doc("m", &leaf, domain.DocMemory, "m", false, t0, body(400)),                               // 100 tok
	}
	// cap=50 < instructions(200)+activeContext(100): both always-tier kept, Used>cap, memory dropped.
	got := usecase.Compose(chain, docs, map[string]bool{}, 50)
	if len(got.Instructions) != 1 || got.ActiveContext == nil {
		t.Fatalf("instructions+activeContext must always load over cap: %+v / %+v", got.Instructions, got.ActiveContext)
	}
	if got.Budget.Used != 300 {
		t.Errorf("Used should be the always-tier sum 300, got %d", got.Budget.Used)
	}
	if len(got.Memories["leaf"]) != 0 || got.Budget.Dropped.Leaf != 1 {
		t.Errorf("the leaf memory must drop, got mem=%+v dropped=%+v", got.Memories["leaf"], got.Budget.Dropped)
	}
}
