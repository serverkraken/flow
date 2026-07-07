package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
	"github.com/serverkraken/flow/internal/usecase"
)

// rankedContextItem is a small test helper building a usecase.RankedItem with
// the fields BuildKontextVM reads (mirrors cockpit_context_vm_test.go's
// rankedItem, extended with ID/Type/ScopeLabel/EstTokens for the rows).
// rank mirrors what usecase.Compose actually assigns (the Included-Rank, 0
// for dropped items) — callers pass 0 for included fixtures that don't care
// about the exact badge number, and the real included-counter otherwise.
func rankedContextItem(id, title string, pinned, included bool) usecase.RankedItem {
	return rankedContextItemRanked(id, title, pinned, included, 0)
}

func rankedContextItemRanked(id, title string, pinned, included bool, rank int) usecase.RankedItem {
	return usecase.RankedItem{
		Item: usecase.ContextItem{
			ID:         id,
			Title:      title,
			Pinned:     pinned,
			Type:       domain.DocMemory,
			ScopeLabel: "repo:flow",
			EstTokens:  1234,
		},
		Included: included,
		Rank:     rank,
	}
}

// TestBuildKontextVM_RowsOrderAndCutline verifies Rows follow cc.Ranked's
// order exactly (no re-sort), FirstDropped marks only the FIRST
// included→dropped transition, and IsFirst/IsLast mark the ends of the whole
// list (not just the included subset).
func TestBuildKontextVM_RowsOrderAndCutline(t *testing.T) {
	n := domain.Node{ID: "n1", Name: "acme/flow"}
	cc := usecase.ComposedContext{
		Budget: usecase.ContextBudget{Used: 9000, Cap: 12000},
		Ranked: []usecase.RankedItem{
			rankedContextItemRanked("d1", "First", true, true, 1),
			rankedContextItemRanked("d2", "Second", false, true, 2),
			rankedContextItemRanked("d3", "Third dropped", false, false, 0),
			rankedContextItemRanked("d4", "Fourth dropped", false, false, 0),
		},
	}

	vm := BuildKontextVM(n, cc)

	if vm.NodeID != "n1" {
		t.Errorf("NodeID = %q, want n1", vm.NodeID)
	}
	if vm.Title != "flow" {
		t.Errorf("Title = %q, want flow (ShortName of acme/flow)", vm.Title)
	}
	if len(vm.Rows) != 4 {
		t.Fatalf("len(Rows) = %d, want 4", len(vm.Rows))
	}
	wantOrder := []string{"d1", "d2", "d3", "d4"}
	wantNum := []int{1, 2, 0, 0} // Included-Rank (usecase.RankedItem.Rank) — 0 for dropped rows
	for i, want := range wantOrder {
		if vm.Rows[i].DocID != want {
			t.Errorf("Rows[%d].DocID = %q, want %q (must preserve cc.Ranked order)", i, vm.Rows[i].DocID, want)
		}
		if vm.Rows[i].Num != wantNum[i] {
			t.Errorf("Rows[%d].Num = %d, want %d", i, vm.Rows[i].Num, wantNum[i])
		}
	}
	if !vm.Rows[0].IsFirst {
		t.Error("Rows[0].IsFirst = false, want true")
	}
	for i := 1; i < len(vm.Rows); i++ {
		if vm.Rows[i].IsFirst {
			t.Errorf("Rows[%d].IsFirst = true, want false", i)
		}
	}
	if !vm.Rows[3].IsLast {
		t.Error("Rows[3].IsLast = false, want true")
	}
	for i := 0; i < 3; i++ {
		if vm.Rows[i].IsLast {
			t.Errorf("Rows[%d].IsLast = true, want false", i)
		}
	}
	// FirstDropped marks exactly ONE row: the first Included=false row (index 2).
	for i, row := range vm.Rows {
		want := i == 2
		if row.FirstDropped != want {
			t.Errorf("Rows[%d].FirstDropped = %v, want %v", i, row.FirstDropped, want)
		}
	}
	if vm.Rows[0].ChipClass != DocTypeChipClass(domain.DocMemory) || vm.Rows[0].TypeLabel != DocTypeLabel(domain.DocMemory) {
		t.Errorf("Rows[0] chip = %q/%q, want DocTypeChipClass/Label(DocMemory)", vm.Rows[0].ChipClass, vm.Rows[0].TypeLabel)
	}
	if vm.Rows[0].ScopeLabel != "repo:flow" {
		t.Errorf("Rows[0].ScopeLabel = %q, want repo:flow", vm.Rows[0].ScopeLabel)
	}
	if vm.Rows[0].TokensStr != "1.234" {
		t.Errorf("Rows[0].TokensStr = %q, want 1.234 (fmtThousandsDE reuse)", vm.Rows[0].TokensStr)
	}
	if !vm.Rows[0].Pinned {
		t.Error("Rows[0].Pinned = false, want true")
	}
}

// TestBuildKontextVM_MeterMatchesCockpit verifies the meter/counter fields
// are derived the same way as BuildCockpitContext (reused, not duplicated).
func TestBuildKontextVM_MeterMatchesCockpit(t *testing.T) {
	n := domain.Node{ID: "n1", Name: "flow"}
	cc := usecase.ComposedContext{
		Budget: usecase.ContextBudget{
			Used: 11891, Cap: 12000,
			Dropped: usecase.DroppedCount{Leaf: 2, Vorhaben: 1, Engagement: 0, Global: 0},
		},
		Ranked: []usecase.RankedItem{
			rankedContextItem("d1", "A", false, true),
			rankedContextItem("d2", "B", false, false),
			rankedContextItem("d3", "C", false, false),
		},
	}
	vm := BuildKontextVM(n, cc)
	if vm.UsedStr != "11.891" || vm.CapStr != "12.000" {
		t.Errorf("UsedStr/CapStr = %q/%q, want 11.891/12.000", vm.UsedStr, vm.CapStr)
	}
	if vm.Pct != 99 || !vm.Full {
		t.Errorf("Pct/Full = %d/%v, want 99/true", vm.Pct, vm.Full)
	}
	if vm.IncludedN != 1 {
		t.Errorf("IncludedN = %d, want 1", vm.IncludedN)
	}
	if vm.DroppedN != 3 {
		t.Errorf("DroppedN = %d, want 3 (2 leaf + 1 vorhaben)", vm.DroppedN)
	}
}

// TestBuildKontextVM_EmptyNoDroppedRow covers the no-memories case: no Rows,
// no FirstDropped anywhere, zeroed counters — the templ renders the quiet
// empty-state line off len(Rows) == 0.
func TestBuildKontextVM_EmptyNoDroppedRow(t *testing.T) {
	n := domain.Node{ID: "n1", Name: "flow"}
	vm := BuildKontextVM(n, usecase.ComposedContext{})
	if len(vm.Rows) != 0 {
		t.Errorf("len(Rows) = %d, want 0", len(vm.Rows))
	}
	if vm.IncludedN != 0 || vm.DroppedN != 0 {
		t.Errorf("IncludedN/DroppedN = %d/%d, want 0/0", vm.IncludedN, vm.DroppedN)
	}
}

// TestBuildKontextVM_AlwaysTier verifies Always is built from cc.Instructions
// + cc.ActiveContext (when set) — the Always-Tier docs that count into the
// budget but never appear as a Ranked row (Mini-Task 6b).
func TestBuildKontextVM_AlwaysTier(t *testing.T) {
	n := domain.Node{ID: "n1", Name: "flow"}
	ac := usecase.ContextItem{
		ID: "ac1", Title: "Active Context", Type: domain.DocActiveContext,
		ScopeLabel: "repo:flow", EstTokens: 456,
	}
	cc := usecase.ComposedContext{
		Instructions: []usecase.ContextItem{
			{ID: "i1", Title: "AGENTS.md", Type: domain.DocInstruction, ScopeLabel: "repo:flow", EstTokens: 1234},
		},
		ActiveContext: &ac,
	}
	vm := BuildKontextVM(n, cc)
	if len(vm.Always) != 2 {
		t.Fatalf("len(Always) = %d, want 2 (1 instruction + active context)", len(vm.Always))
	}
	inst := vm.Always[0]
	if inst.Title != "AGENTS.md" {
		t.Errorf("Always[0].Title = %q, want AGENTS.md", inst.Title)
	}
	if inst.ChipClass != DocTypeChipClass(domain.DocInstruction) || inst.TypeLabel != DocTypeLabel(domain.DocInstruction) {
		t.Errorf("Always[0] chip = %q/%q, want DocTypeChipClass/Label(DocInstruction)", inst.ChipClass, inst.TypeLabel)
	}
	if inst.ScopeLabel != "repo:flow" {
		t.Errorf("Always[0].ScopeLabel = %q, want repo:flow", inst.ScopeLabel)
	}
	if inst.TokensStr != "1.234" {
		t.Errorf("Always[0].TokensStr = %q, want 1.234", inst.TokensStr)
	}
	ac2 := vm.Always[1]
	if ac2.Title != "Active Context" || ac2.TypeLabel != DocTypeLabel(domain.DocActiveContext) {
		t.Errorf("Always[1] = %+v, want Active Context / %s", ac2, DocTypeLabel(domain.DocActiveContext))
	}
	if ac2.TokensStr != "456" {
		t.Errorf("Always[1].TokensStr = %q, want 456", ac2.TokensStr)
	}

	// No Instructions, no ActiveContext → empty Always (section must not render).
	empty := BuildKontextVM(n, usecase.ComposedContext{})
	if len(empty.Always) != 0 {
		t.Errorf("len(Always) = %d, want 0 for empty ComposedContext", len(empty.Always))
	}
}

// TestKontextFragment_AlwaysSection_PresentWhenSet verifies the Always-Tier
// section (eyebrow + non-curatable rows, no rank/pin/reorder) renders above
// the rang list when vm.Always is populated (Mini-Task 6b). Mini-Task 7c adds
// a direct Bearbeiten-link as the row's only action — pin/reorder stay absent.
func TestKontextFragment_AlwaysSection_PresentWhenSet(t *testing.T) {
	vm := KontextVM{
		NodeID: "n1", Title: "flow",
		Always: []KontextAlwaysVM{
			{DocID: "i1", Title: "AGENTS.md", ChipClass: DocTypeChipClass(domain.DocInstruction), TypeLabel: DocTypeLabel(domain.DocInstruction), ScopeLabel: "repo:flow", TokensStr: "1.234"},
			{DocID: "ac1", Title: "Active Context", ChipClass: DocTypeChipClass(domain.DocActiveContext), TypeLabel: DocTypeLabel(domain.DocActiveContext), ScopeLabel: "repo:flow", TokensStr: "456"},
		},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, KontextFragment(vm))
	for _, want := range []string{
		"Immer enthalten — Instruktionen &amp; Active Context",
		"AGENTS.md", "Active Context", "immer enthalten",
		`href="/wissen/i1/bearbeiten"`, `href="/wissen/ac1/bearbeiten"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("always section misses %q:\n%s", want, out)
		}
	}
	// Always rows carry no rank badge and no pin/reorder actions (not curatable) —
	// only the Bearbeiten-link (btn-q styled, Mini-Task 7c).
	for _, unwanted := range []string{"↑", "↓"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("always section must render no pin/reorder controls, found %q:\n%s", unwanted, out)
		}
	}
}

// TestKontextFragment_AlwaysSection_AbsentWhenEmpty verifies the empty
// Always-Tier (no Instructions, no ActiveContext) renders no section at all.
func TestKontextFragment_AlwaysSection_AbsentWhenEmpty(t *testing.T) {
	vm := KontextVM{NodeID: "n1", Title: "flow"}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, KontextFragment(vm))
	if strings.Contains(out, "Immer enthalten — Instruktionen &amp; Active Context") {
		t.Fatalf("empty Always must render no section:\n%s", out)
	}
}

// TestKontextFragment_RowsAndAlwaysLinkToDocument verifies both the rang-list
// rows and the Always-Tier rows link their title to the document page
// (/wissen/{id}), so Kuratieren can jump straight to Bearbeiten without going
// via Wissen (Mini-Task 7b).
func TestKontextFragment_RowsAndAlwaysLinkToDocument(t *testing.T) {
	vm := KontextVM{
		NodeID: "n1", Title: "flow",
		Always: []KontextAlwaysVM{
			{DocID: "ac1", Title: "Active Context", ChipClass: DocTypeChipClass(domain.DocActiveContext), TypeLabel: DocTypeLabel(domain.DocActiveContext), ScopeLabel: "repo:flow", TokensStr: "456"},
		},
		Rows: []KontextRowVM{
			{DocID: "d1", Num: 1, Title: "First", ChipClass: DocTypeChipClass(domain.DocMemory), TypeLabel: DocTypeLabel(domain.DocMemory), ScopeLabel: "repo:flow", TokensStr: "1.234", IsFirst: true, IsLast: true, Included: true},
		},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, KontextFragment(vm))
	for _, want := range []string{
		`href="/wissen/ac1"`,
		`href="/wissen/d1"`,
		// Mini-Task 7c: each row also carries a direct Bearbeiten-link to the
		// editor route (/wissen/{id}/bearbeiten), next to the title link.
		`href="/wissen/ac1/bearbeiten"`,
		`href="/wissen/d1/bearbeiten"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("fragment misses %q:\n%s", want, out)
		}
	}
}

// TestBuildKontextVM_NumMatchesStandingOfRank is the Final-Review F1
// regression: on a real (non-monotone) budget overflow — one large doc
// dropped, a later smaller doc included right after it — the Kuratieren
// badge (vm.Rows[i].Num) must equal usecase.StandingOf's Included-Rank for
// the SAME document, and the dropped row must carry no number (Num == 0,
// rendered as "—" by kontext.templ) rather than a stale positional index.
func TestBuildKontextVM_NumMatchesStandingOfRank(t *testing.T) {
	leaf := "L"
	chain := []domain.Node{{ID: leaf, Name: "flow", Slug: "flow", Kind: domain.KindRepo}}
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	body := func(n int) string { return strings.Repeat("x", n) } // EstTokens = ceil(n/4)
	docs := []domain.Document{
		// big, newer → ranked first (newest-first tiebreak) but does NOT fit cap=150.
		{ID: "big", NodeID: &leaf, Type: domain.DocMemory, Path: "big", UpdatedAt: t0.Add(time.Hour), Body: body(800)},
		// small, older → ranked second, fits the remaining cap.
		{ID: "small", NodeID: &leaf, Type: domain.DocMemory, Path: "small", UpdatedAt: t0, Body: body(40)},
	}
	cc := usecase.Compose(chain, docs, map[string]bool{}, 150)

	n := domain.Node{ID: leaf, Name: "flow"}
	vm := BuildKontextVM(n, cc)
	if len(vm.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(vm.Rows))
	}
	big, small := vm.Rows[0], vm.Rows[1]
	if big.DocID != "big" || big.Included {
		t.Fatalf("Rows[0] = %+v, want dropped doc %q", big, "big")
	}
	if big.Num != 0 {
		t.Errorf("dropped row Num = %d, want 0 (no rank badge)", big.Num)
	}
	if small.DocID != "small" || !small.Included {
		t.Fatalf("Rows[1] = %+v, want included doc %q", small, "small")
	}

	standing := usecase.StandingOf(cc, "small")
	if standing.State != "included" {
		t.Fatalf("StandingOf(small).State = %q, want included", standing.State)
	}
	if small.Num != standing.Rank {
		t.Errorf("Kuratieren badge Num = %d, StandingOf Rank = %d — must match (F1)", small.Num, standing.Rank)
	}
	if small.Num != 1 {
		t.Errorf("small.Num = %d, want 1 (only included doc)", small.Num)
	}
}

// TestKontextFragment_DroppedRowShowsNoNumberBadge verifies the templ output:
// an included row (Num>0) shows the "%02d" rank badge, a dropped row
// (Included=false, Num=0) shows a dash instead of a stale/zero number.
func TestKontextFragment_DroppedRowShowsNoNumberBadge(t *testing.T) {
	vm := KontextVM{
		NodeID: "n1", Title: "flow",
		Rows: []KontextRowVM{
			{DocID: "d1", Num: 1, Title: "Included", TypeLabel: DocTypeLabel(domain.DocMemory), ScopeLabel: "repo:flow", TokensStr: "10", Included: true, IsFirst: true},
			{DocID: "d2", Num: 0, Title: "Dropped", TypeLabel: DocTypeLabel(domain.DocMemory), ScopeLabel: "repo:flow", TokensStr: "10", Included: false, FirstDropped: true, IsLast: true},
		},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, KontextFragment(vm))
	if !strings.Contains(out, `<span class="num">01</span>`) {
		t.Fatalf("included row must show 01 rank badge:\n%s", out)
	}
	if !strings.Contains(out, `<span class="num text-faint">—</span>`) {
		t.Fatalf("dropped row must show a dash, not a number:\n%s", out)
	}
	if strings.Contains(out, `<span class="num">00</span>`) {
		t.Fatalf("dropped row must never render Num=0 as 00:\n%s", out)
	}
}

// TestBuildKontextVM_AllIncludedNoCutline covers the case where every row is
// included: FirstDropped must never fire (no cutline row exists).
func TestBuildKontextVM_AllIncludedNoCutline(t *testing.T) {
	n := domain.Node{ID: "n1", Name: "flow"}
	cc := usecase.ComposedContext{
		Ranked: []usecase.RankedItem{
			rankedContextItem("d1", "A", false, true),
			rankedContextItem("d2", "B", false, true),
		},
	}
	vm := BuildKontextVM(n, cc)
	for i, row := range vm.Rows {
		if row.FirstDropped {
			t.Errorf("Rows[%d].FirstDropped = true, want false (all included)", i)
		}
	}
}
