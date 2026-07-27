package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

// rankedItem is a small test helper building a usecase.RankedItem with just
// the fields BuildCockpitContext reads (Item.Title/Pinned + Included).
func rankedItem(title string, pinned, included bool) usecase.RankedItem {
	return usecase.RankedItem{
		Item:     usecase.ContextItem{Title: title, Pinned: pinned},
		Included: included,
	}
}

// TestBuildCockpitContext_MeterAndCounters verifies the Budget-derived meter
// (Pct/Full/UsedStr/CapStr) and the Ranked-derived counters (Included/
// Dropped/Pinned) from a hand-built ComposedContext — no domain/store I/O.
func TestBuildCockpitContext_MeterAndCounters(t *testing.T) {
	cc := usecase.ComposedContext{
		Budget: usecase.ContextBudget{
			Used: 11891, Cap: 12000,
			Dropped: usecase.DroppedCount{Leaf: 20, Vorhaben: 15, Engagement: 10, Global: 20, Pinned: 3},
		},
		Ranked: []usecase.RankedItem{
			rankedItem("Tailwind v4 + templ Gotchas", true, true),
			rankedItem("Plans need a main-wiring task", true, true),
			rankedItem("Keine Monolithen", true, true),
			rankedItem("Charm v2 Width Gotcha", true, true), // 4th pinned+included — must NOT appear in TopPins (cap 3)
			rankedItem("some unpinned included memory", false, true),
			rankedItem("a dropped pinned memory", true, false), // pinned but dropped — counts toward PinnedN, not TopPins
			rankedItem("a dropped unpinned memory", false, false),
		},
	}

	vm := BuildCockpitContext(cc, "node-1")
	if vm == nil {
		t.Fatal("BuildCockpitContext must never return nil")
	}
	if vm.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1", vm.NodeID)
	}
	if vm.UsedStr != "11.891" {
		t.Errorf("UsedStr = %q, want 11.891", vm.UsedStr)
	}
	if vm.CapStr != "12.000" {
		t.Errorf("CapStr = %q, want 12.000", vm.CapStr)
	}
	if vm.Pct != 99 {
		t.Errorf("Pct = %d, want 99 (11891*100/12000 truncated)", vm.Pct)
	}
	if !vm.Full {
		t.Error("Full = false, want true (Pct >= 95)")
	}
	// IncludedN counts len(Ranked.Included) — 5 items have Included=true above.
	if vm.IncludedN != 5 {
		t.Errorf("IncludedN = %d, want 5", vm.IncludedN)
	}
	// DroppedN is Σ Budget.Dropped(Leaf+Vorhaben+Engagement+Global), NOT +Pinned
	// (Pinned double-counts an already-bucketed drop).
	if vm.DroppedN != 65 {
		t.Errorf("DroppedN = %d, want 65 (20+15+10+20)", vm.DroppedN)
	}
	// PinnedN is context-scoped: every Ranked item with Item.Pinned, included
	// or dropped (5 pinned items in the fixture above: 4 included + 1 dropped).
	if vm.PinnedN != 5 {
		t.Errorf("PinnedN = %d, want 5", vm.PinnedN)
	}
	if len(vm.TopPins) != 3 {
		t.Fatalf("TopPins len = %d, want 3 (capped)", len(vm.TopPins))
	}
	wantPins := []ContextPinVM{
		{Num: "01", Title: "Tailwind v4 + templ Gotchas"},
		{Num: "02", Title: "Plans need a main-wiring task"},
		{Num: "03", Title: "Keine Monolithen"},
	}
	for i, want := range wantPins {
		if vm.TopPins[i] != want {
			t.Errorf("TopPins[%d] = %+v, want %+v", i, vm.TopPins[i], want)
		}
	}
}

// TestBuildCockpitContext_EmptyAndClamped covers the empty state (no docs at
// all → 0% meter, zero counters, no pins) and Pct clamping at both ends
// (cap<=0 degrades to 0%, an over-budget Used never exceeds 100%).
func TestBuildCockpitContext_EmptyAndClamped(t *testing.T) {
	empty := BuildCockpitContext(usecase.ComposedContext{}, "n")
	if empty.Pct != 0 || empty.Full || empty.IncludedN != 0 || empty.DroppedN != 0 || empty.PinnedN != 0 || len(empty.TopPins) != 0 {
		t.Errorf("empty context should be all-zero, got %+v", empty)
	}
	if empty.UsedStr != "0" || empty.CapStr != "0" {
		t.Errorf("empty UsedStr/CapStr = %q/%q, want 0/0", empty.UsedStr, empty.CapStr)
	}

	over := BuildCockpitContext(usecase.ComposedContext{Budget: usecase.ContextBudget{Used: 500, Cap: 100}}, "n")
	if over.Pct != 100 {
		t.Errorf("over-budget Pct = %d, want clamped 100", over.Pct)
	}
	if !over.Full {
		t.Error("over-budget Full = false, want true")
	}
}

// TestBuildCockpitContext_AlwaysN verifies AlwaysN counts Instructions plus
// ActiveContext (when set) — the Always-Tier docs that count into
// Budget.Used but never appear as a Ranked row (Mini-Task 6b: Soenne stumbled
// on "2.766 tk bei 0 Docs" live).
func TestBuildCockpitContext_AlwaysN(t *testing.T) {
	ac := usecase.ContextItem{Title: "Active Context"}
	cc := usecase.ComposedContext{
		Instructions:  []usecase.ContextItem{{Title: "AGENTS.md"}, {Title: "CLAUDE.md"}},
		ActiveContext: &ac,
	}
	vm := BuildCockpitContext(cc, "node-1")
	if vm.AlwaysN != 3 {
		t.Errorf("AlwaysN = %d, want 3 (2 instructions + 1 active context)", vm.AlwaysN)
	}

	noActive := usecase.ComposedContext{
		Instructions: []usecase.ContextItem{{Title: "AGENTS.md"}},
	}
	vm2 := BuildCockpitContext(noActive, "node-1")
	if vm2.AlwaysN != 1 {
		t.Errorf("AlwaysN = %d, want 1 (no active context set)", vm2.AlwaysN)
	}

	empty := BuildCockpitContext(usecase.ComposedContext{}, "node-1")
	if empty.AlwaysN != 0 {
		t.Errorf("AlwaysN = %d, want 0", empty.AlwaysN)
	}
}

// TestBuildCockpitContext_AlwaysN_IncludesAlwaysMemories is L5.5 Task 4's
// regression: an "immer"-mode memory forced into cc.AlwaysMemories must count
// toward the rail panel's "Immer enthalten" line just like Instructions/
// ActiveContext — otherwise the panel undercounts what Budget.Used already
// includes (Compose already adds AlwaysMemories tokens to Used, Task 2).
func TestBuildCockpitContext_AlwaysN_IncludesAlwaysMemories(t *testing.T) {
	cc := usecase.ComposedContext{
		Instructions:   []usecase.ContextItem{{Title: "AGENTS.md"}},
		AlwaysMemories: []usecase.ContextItem{{Title: "Pinned-forever memory"}, {Title: "Another immer memory"}},
	}
	vm := BuildCockpitContext(cc, "node-1")
	if vm.AlwaysN != 3 {
		t.Errorf("AlwaysN = %d, want 3 (1 instruction + 2 immer memories)", vm.AlwaysN)
	}
}

func TestFmtThousandsDE(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1.000"},
		{11891, "11.891"},
		{12000, "12.000"},
		{1234567, "1.234.567"},
	}
	for _, c := range cases {
		if got := fmtThousandsDE(c.in); got != c.want {
			t.Errorf("fmtThousandsDE(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
