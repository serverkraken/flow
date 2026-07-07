package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

// TestBuildDocContext_Included covers the "enthalten" state: RankStr formats
// as "04 / 24" (zero-padded), Included is true, NodeName passes through.
func TestBuildDocContext_Included(t *testing.T) {
	st := usecase.ContextStanding{State: "included", Rank: 4, Total: 24}
	vm := BuildDocContext(st, "backstage")
	if vm == nil {
		t.Fatal("BuildDocContext must not return nil for included")
	}
	if vm.State != "included" {
		t.Errorf("State = %q, want included", vm.State)
	}
	if !vm.Included {
		t.Error("Included = false, want true")
	}
	if vm.RankStr != "04 / 24" {
		t.Errorf("RankStr = %q, want %q", vm.RankStr, "04 / 24")
	}
	if vm.NodeName != "backstage" {
		t.Errorf("NodeName = %q, want backstage", vm.NodeName)
	}
}

// TestBuildDocContext_Dropped covers the "verworfen" state: no RankStr,
// Included false.
func TestBuildDocContext_Dropped(t *testing.T) {
	st := usecase.ContextStanding{State: "dropped"}
	vm := BuildDocContext(st, "backstage")
	if vm == nil {
		t.Fatal("BuildDocContext must not return nil for dropped")
	}
	if vm.State != "dropped" {
		t.Errorf("State = %q, want dropped", vm.State)
	}
	if vm.Included {
		t.Error("Included = true, want false")
	}
	if vm.RankStr != "" {
		t.Errorf("RankStr = %q, want empty for dropped", vm.RankStr)
	}
}

// TestBuildDocContext_Always covers Instructions/ActiveContext ("immer
// enthalten") — no rank (they never enter the ranked pool).
func TestBuildDocContext_Always(t *testing.T) {
	st := usecase.ContextStanding{State: "always"}
	vm := BuildDocContext(st, "backstage")
	if vm == nil {
		t.Fatal("BuildDocContext must not return nil for always")
	}
	if vm.State != "always" {
		t.Errorf("State = %q, want always", vm.State)
	}
	if vm.RankStr != "" {
		t.Errorf("RankStr = %q, want empty for always", vm.RankStr)
	}
}

// TestBuildDocContext_Absent covers the "no block" case: a non-context-type
// doc, or a context-type doc not present in the composed chain at all.
func TestBuildDocContext_Absent(t *testing.T) {
	st := usecase.ContextStanding{State: "absent"}
	vm := BuildDocContext(st, "backstage")
	if vm != nil {
		t.Fatalf("BuildDocContext = %+v, want nil for absent", vm)
	}
}
