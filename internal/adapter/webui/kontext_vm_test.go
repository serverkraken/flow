package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// rankedContextItem is a small test helper building a usecase.RankedItem with
// the fields BuildKontextVM reads (mirrors cockpit_context_vm_test.go's
// rankedItem, extended with ID/Type/ScopeLabel/EstTokens for the rows).
func rankedContextItem(id, title string, pinned, included bool) usecase.RankedItem {
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
			rankedContextItem("d1", "First", true, true),
			rankedContextItem("d2", "Second", false, true),
			rankedContextItem("d3", "Third dropped", false, false),
			rankedContextItem("d4", "Fourth dropped", false, false),
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
	for i, want := range wantOrder {
		if vm.Rows[i].DocID != want {
			t.Errorf("Rows[%d].DocID = %q, want %q (must preserve cc.Ranked order)", i, vm.Rows[i].DocID, want)
		}
		if vm.Rows[i].Num != i+1 {
			t.Errorf("Rows[%d].Num = %d, want %d", i, vm.Rows[i].Num, i+1)
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
