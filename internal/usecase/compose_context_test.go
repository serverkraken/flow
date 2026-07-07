package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
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

// TestCompose_PriorityLiftsAcrossTier: a leaf memory with Priority=5 must fill
// before an unprioritized engagement memory, even though tierRank alone would
// put engagement (rank 1) ahead of leaf (rank 3). Same pin status (both
// unpinned) so priority is the deciding key.
func TestCompose_PriorityLiftsAcrossTier(t *testing.T) {
	t.Parallel()
	leaf, eng := "L", "E"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo), node(eng, "Privat", domain.KindEngagement)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "leafPrio", NodeID: &leaf, Type: domain.DocMemory, Path: "l", Priority: 5, UpdatedAt: t0, Body: "leaf, prioritized"},
		{ID: "engPlain", NodeID: &eng, Type: domain.DocMemory, Path: "e", Priority: 0, UpdatedAt: t0, Body: "engagement, default prio"},
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)
	if len(got.Ranked) != 2 {
		t.Fatalf("want 2 ranked items, got %d: %+v", len(got.Ranked), got.Ranked)
	}
	if got.Ranked[0].Item.ID != "leafPrio" {
		t.Fatalf("priority must lift the leaf memory ahead of the engagement memory: %+v", got.Ranked)
	}
	if got.Ranked[1].Item.ID != "engPlain" {
		t.Fatalf("engagement memory should follow: %+v", got.Ranked)
	}
}

// TestCompose_RankedFlatOrder: Ranked mirrors the pool's global fill order;
// Included items get a contiguous 1..N rank, dropped items get Included=false
// and Rank=0. Priority does NOT bypass the cap (only pinned does): a
// high-priority item that does not fit still drops.
func TestCompose_RankedFlatOrder(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // EstTokens = ceil(n/4)
	docs := []domain.Document{
		{ID: "hiPrioBig", NodeID: &leaf, Type: domain.DocMemory, Path: "a", Priority: 9, UpdatedAt: t0, Body: body(400)}, // 100 tok, fills first but too big alone with the next
		{ID: "loPrioSmall", NodeID: &leaf, Type: domain.DocMemory, Path: "b", Priority: 1, UpdatedAt: t0, Body: body(400)},
	}
	// cap=100 → only the first (highest priority) fits; the second drops.
	got := usecase.Compose(chain, docs, map[string]bool{}, 100)
	if len(got.Ranked) != 2 {
		t.Fatalf("want 2 ranked items (one included, one dropped), got %+v", got.Ranked)
	}
	if got.Ranked[0].Item.ID != "hiPrioBig" || !got.Ranked[0].Included || got.Ranked[0].Rank != 1 {
		t.Fatalf("first item should be included at rank 1: %+v", got.Ranked[0])
	}
	if got.Ranked[1].Item.ID != "loPrioSmall" || got.Ranked[1].Included || got.Ranked[1].Rank != 0 {
		t.Fatalf("second item should be dropped (priority does not bypass the cap): %+v", got.Ranked[1])
	}
}

// TestCompose_ZeroPriorityIsBestandOrder: with every doc at the Priority
// zero-value, the new (pinned, priority, tierRank, updatedAt) key must
// degenerate to the pre-L5 (pinned, tierRank, updatedAt) order — this is the
// same fixture/assertions as TestCompose_TierRankFillOrder, restated to pin
// down backward compatibility explicitly.
func TestCompose_ZeroPriorityIsBestandOrder(t *testing.T) {
	t.Parallel()
	leaf, vor, eng := "L", "V", "E"
	chain := []domain.Node{
		node(leaf, "flow", domain.KindRepo),
		node(vor, "Vorhaben", domain.KindVorhaben),
		node(eng, "Privat", domain.KindEngagement),
	}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("le", &leaf, domain.DocMemory, "l", false, t0, body(400)),
		doc("vo", &vor, domain.DocMemory, "v", false, t0, body(400)),
		doc("en", &eng, domain.DocMemory, "e", false, t0, body(400)),
		doc("gl", nil, domain.DocMemory, "g", false, t0, body(400)),
	}
	for _, d := range docs {
		if d.Priority != 0 {
			t.Fatalf("fixture must use the Priority zero-value: %+v", d)
		}
	}
	got := usecase.Compose(chain, docs, map[string]bool{"gl": true}, 250)
	if len(got.Memories["global"]) != 1 || len(got.Memories["engagement"]) != 1 {
		t.Fatalf("global+engagement should survive tight cap unchanged: %+v", got.Memories)
	}
	if len(got.Memories["vorhaben"]) != 0 || len(got.Memories["leaf"]) != 0 {
		t.Fatalf("vorhaben+leaf should still drop: %+v", got.Memories)
	}
	if got.Budget.Dropped.Vorhaben != 1 || got.Budget.Dropped.Leaf != 1 {
		t.Errorf("want vorhaben=1 leaf=1 dropped, got %+v", got.Budget.Dropped)
	}
}

func TestStandingOf_States(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) }
	docs := []domain.Document{
		doc("instr", &leaf, domain.DocInstruction, "claude", false, t0, "rule"),
		doc("ac", &leaf, domain.DocActiveContext, usecase.ActiveContextPath, false, t0, "where"),
		doc("kept", &leaf, domain.DocMemory, "k", false, t0, body(4)),
		doc("gone", &leaf, domain.DocMemory, "g", false, t0, body(400)),
	}
	// cap fits instructions(~1tok)+activeContext(~1tok)+kept(1tok) but not gone(100tok).
	got := usecase.Compose(chain, docs, map[string]bool{}, 10)

	if st := usecase.StandingOf(got, "instr"); st.State != "always" {
		t.Errorf("instruction should be always, got %+v", st)
	}
	if st := usecase.StandingOf(got, "ac"); st.State != "always" {
		t.Errorf("activeContext should be always, got %+v", st)
	}
	if st := usecase.StandingOf(got, "kept"); st.State != "included" || st.Rank != 1 || st.Total != 1 {
		t.Errorf("kept memory should be included rank 1/1, got %+v", st)
	}
	if st := usecase.StandingOf(got, "gone"); st.State != "dropped" {
		t.Errorf("dropped memory should be dropped, got %+v", st)
	}
	if st := usecase.StandingOf(got, "nonexistent"); st.State != "absent" {
		t.Errorf("unknown doc should be absent, got %+v", st)
	}
}

// TestComposeContext_ExecuteForNode: composing by node ID must yield the same
// result as resolving the same leaf via the binding registry.
func TestComposeContext_ExecuteForNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docsStore := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	eng, _ := nodes.Create(ctx, domain.Node{ID: "E", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Privat", Slug: "privat"})
	leaf, _ := nodes.Create(ctx, domain.Node{ID: "L", OwnerID: "u1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", ParentID: &eng.ID, OriginSlug: "flow"})
	_, _ = binds.Upsert(ctx, domain.ProjectBinding{Kind: domain.BindingRemote, OwnerID: "u1", RemoteSlug: "flow", NodeID: leaf.ID})

	t0 := time.Now()
	_, _ = docsStore.Create(ctx, domain.Document{ID: "ac", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocActiveContext, Path: usecase.ActiveContextPath, Body: "where", UpdatedAt: t0})
	_, _ = docsStore.Create(ctx, domain.Document{ID: "m", OwnerID: "u1", NodeID: &leaf.ID, Type: domain.DocMemory, Path: "m", Body: "mem", UpdatedAt: t0})

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docsStore, Tags: tags,
	}
	viaSlug, err := uc.Execute(ctx, "u1", usecase.ContextResolveInput{RemoteSlug: "flow"}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	viaID, err := uc.ExecuteForNode(ctx, "u1", leaf.ID, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if viaID.Resolution.Unresolved {
		t.Fatalf("ExecuteForNode should resolve: %+v", viaID.Resolution)
	}
	if viaID.ActiveContext == nil || viaID.ActiveContext.ID != viaSlug.ActiveContext.ID {
		t.Errorf("ExecuteForNode activeContext mismatch: %+v vs %+v", viaID.ActiveContext, viaSlug.ActiveContext)
	}
	if len(viaID.Memories["leaf"]) != len(viaSlug.Memories["leaf"]) {
		t.Errorf("ExecuteForNode leaf memories mismatch: %+v vs %+v", viaID.Memories["leaf"], viaSlug.Memories["leaf"])
	}
}

// TestComposeContext_ExecuteForNode_ForeignNode: an owner-scoped
// FakeNodeStore.Get on a foreign owner's node returns ports.ErrNodeNotFound;
// ExecuteForNode must propagate it and surface no foreign docs (Codex-Fund #2).
func TestComposeContext_ExecuteForNode_ForeignNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	docsStore := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	binds := testutil.NewFakeProjectBindingStore()

	foreign, _ := nodes.Create(ctx, domain.Node{ID: "F", OwnerID: "u2", Kind: domain.KindRepo, Name: "secret", Slug: "secret"})
	_, _ = docsStore.Create(ctx, domain.Document{ID: "secretmem", OwnerID: "u2", NodeID: &foreign.ID, Type: domain.DocMemory, Path: "s", Body: "top secret"})

	uc := usecase.ComposeContext{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes, Docs: docsStore, Tags: tags,
	}
	got, err := uc.ExecuteForNode(ctx, "u1", foreign.ID, 100000)
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ports.ErrNodeNotFound for a foreign node, got %v", err)
	}
	for _, mems := range got.Memories {
		for _, m := range mems {
			if m.ID == "secretmem" {
				t.Fatalf("foreign owner's document must not leak into ComposedContext")
			}
		}
	}
}

// TestCompose_ImmerAlwaysUncapped: a memory doc marked ContextModeImmer lands
// in AlwaysMemories, is counted in Budget.Used, and is never dropped even
// under a cap far too small to hold it — the always-tier is uncapped.
func TestCompose_ImmerAlwaysUncapped(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return string(make([]byte, n)) } // 400 bytes → 100 tok
	docs := []domain.Document{
		{ID: "immerMem", NodeID: &leaf, Type: domain.DocMemory, Path: "m", ContextMode: domain.ContextModeImmer, UpdatedAt: t0, Body: body(400)},
	}
	// cap=1 is far too small for the 100-tok doc; immer must still be kept.
	got := usecase.Compose(chain, docs, map[string]bool{}, 1)
	if len(got.AlwaysMemories) != 1 || got.AlwaysMemories[0].ID != "immerMem" {
		t.Fatalf("immer memory must land in AlwaysMemories, got %+v", got.AlwaysMemories)
	}
	if got.Budget.Used != 100 {
		t.Errorf("immer memory must count in Budget.Used, got %d", got.Budget.Used)
	}
	if len(got.Memories["leaf"]) != 0 {
		t.Errorf("immer memory must NOT appear in the pooled Memories, got %+v", got.Memories["leaf"])
	}
	if len(got.Ranked) != 0 {
		t.Errorf("immer memory must NOT appear in Ranked, got %+v", got.Ranked)
	}
}

// TestCompose_ImmerGlobalBypassesGate: an immer global memory is included
// with NEITHER globalAllowed NOR Pinned set — immer bypasses the D7 tag-gate
// and the pin mechanism entirely (Soenne's "8 globals" fix).
func TestCompose_ImmerGlobalBypassesGate(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "immerGlobal", NodeID: nil, Type: domain.DocMemory, Path: "g", ContextMode: domain.ContextModeImmer, Pinned: false, UpdatedAt: t0, Body: "x"},
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000) // empty globalAllowed, not pinned
	if len(got.AlwaysMemories) != 1 || got.AlwaysMemories[0].ID != "immerGlobal" {
		t.Fatalf("immer global memory must bypass the tag-gate/pin, got %+v", got.AlwaysMemories)
	}
	if len(got.Memories["global"]) != 0 {
		t.Errorf("immer global memory must NOT be in the pooled global memories, got %+v", got.Memories["global"])
	}
}

// TestCompose_NieHiddenNotComposed: a nie-Doc (memory or instruction) is
// collected in Hidden only, never in Used/Ranked/Memories/Instructions/Always.
func TestCompose_NieHiddenNotComposed(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "nieMem", NodeID: &leaf, Type: domain.DocMemory, Path: "m", ContextMode: domain.ContextModeNie, UpdatedAt: t0, Body: "hidden"},
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)
	if len(got.Hidden) != 1 || got.Hidden[0].ID != "nieMem" {
		t.Fatalf("nie memory must land in Hidden, got %+v", got.Hidden)
	}
	if got.Budget.Used != 0 {
		t.Errorf("nie doc must not count in Budget.Used, got %d", got.Budget.Used)
	}
	if len(got.Memories["leaf"]) != 0 {
		t.Errorf("nie doc must not be pooled, got %+v", got.Memories["leaf"])
	}
	if len(got.Ranked) != 0 {
		t.Errorf("nie doc must not appear in Ranked, got %+v", got.Ranked)
	}
	if len(got.AlwaysMemories) != 0 {
		t.Errorf("nie doc must not appear in AlwaysMemories, got %+v", got.AlwaysMemories)
	}
}

// TestCompose_NieDemotesInstruction: an instruction marked nie is degraded to
// Hidden instead of Instructions — Soenne can silence an instruction outright.
func TestCompose_NieDemotesInstruction(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "nieInstr", NodeID: &leaf, Type: domain.DocInstruction, Path: "claude", ContextMode: domain.ContextModeNie, UpdatedAt: t0, Body: "silenced rule"},
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)
	if len(got.Instructions) != 0 {
		t.Fatalf("nie instruction must NOT appear in Instructions, got %+v", got.Instructions)
	}
	if len(got.Hidden) != 1 || got.Hidden[0].ID != "nieInstr" {
		t.Fatalf("nie instruction must land in Hidden, got %+v", got.Hidden)
	}
}

// TestCompose_AutoIsBestandNeutral: every doc left at the auto/zero-value
// ContextMode must produce Memories+Ranked+Used IDENTICAL to the pre-L5.5
// behavior — AlwaysMemories/Hidden stay empty. This restates
// TestCompose_TiersAndActiveContext to pin down default-neutrality explicitly.
func TestCompose_AutoIsBestandNeutral(t *testing.T) {
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
	for _, d := range docs {
		if d.ContextMode != "" {
			t.Fatalf("fixture must use the ContextMode zero-value: %+v", d)
		}
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
	if len(got.AlwaysMemories) != 0 {
		t.Errorf("auto path must leave AlwaysMemories empty, got %+v", got.AlwaysMemories)
	}
	if len(got.Hidden) != 0 {
		t.Errorf("auto path must leave Hidden empty, got %+v", got.Hidden)
	}
	for _, it := range got.Instructions {
		if it.ContextMode != domain.ContextModeAuto {
			t.Errorf("ContextItem.ContextMode must default to auto, got %q for %s", it.ContextMode, it.ID)
		}
	}
}

// TestStandingOf_ImmerMemoryAlways: an immer memory's standing is "always"
// (mirroring instructions/activeContext); a nie-doc is absent (StandingOf
// stays independent of Hidden by design — the doc side upgrades it).
func TestStandingOf_ImmerMemoryAlways(t *testing.T) {
	t.Parallel()
	leaf := "L"
	chain := []domain.Node{node(leaf, "flow", domain.KindRepo)}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "immerMem", NodeID: &leaf, Type: domain.DocMemory, Path: "m", ContextMode: domain.ContextModeImmer, UpdatedAt: t0, Body: "x"},
		{ID: "nieMem", NodeID: &leaf, Type: domain.DocMemory, Path: "n", ContextMode: domain.ContextModeNie, UpdatedAt: t0, Body: "y"},
	}
	got := usecase.Compose(chain, docs, map[string]bool{}, 100000)
	if st := usecase.StandingOf(got, "immerMem"); st.State != "always" {
		t.Errorf("immer memory should be always, got %+v", st)
	}
	if st := usecase.StandingOf(got, "nieMem"); st.State != "absent" {
		t.Errorf("nie doc should be absent from StandingOf, got %+v", st)
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
