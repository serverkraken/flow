package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestBuildDocContext_Included covers the "enthalten" state: RankStr formats
// as "04 / 24" (zero-padded), Included is true, NodeName passes through, and
// the auto mode passed in is carried onto the VM for the mode switcher.
func TestBuildDocContext_Included(t *testing.T) {
	st := usecase.ContextStanding{State: "included", Rank: 4, Total: 24}
	vm := BuildDocContext(st, "backstage", domain.ContextModeAuto)
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
	if vm.Mode != domain.ContextModeAuto {
		t.Errorf("Mode = %q, want auto", vm.Mode)
	}
}

// TestBuildDocContext_Dropped covers the "verworfen" state: no RankStr,
// Included false.
func TestBuildDocContext_Dropped(t *testing.T) {
	st := usecase.ContextStanding{State: "dropped"}
	vm := BuildDocContext(st, "backstage", domain.ContextModeAuto)
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
	vm := BuildDocContext(st, "backstage", domain.ContextModeAuto)
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

// TestBuildDocContext_AbsentStillReturnsBlock is L5.5 Task 4's behavior
// change: a context-eligible doc absent from the composed chain (or a
// Compose failure upstream) used to render NO block at all. Now the block
// must always be built for context types so the mode switcher stays
// reachable even when the doc currently composes to nothing (Codex-Fund /
// brief interface note: "gibt für Kontext-Typen immer einen Block zurück,
// nie nil").
func TestBuildDocContext_AbsentStillReturnsBlock(t *testing.T) {
	st := usecase.ContextStanding{State: "absent"}
	vm := BuildDocContext(st, "backstage", domain.ContextModeAuto)
	if vm == nil {
		t.Fatal("BuildDocContext must not return nil anymore, even for absent — the mode switcher must stay reachable")
	}
	if vm.State != "absent" {
		t.Errorf("State = %q, want absent", vm.State)
	}
	if vm.Included {
		t.Error("Included = true, want false for absent")
	}
}

// TestBuildDocContext_NieModeCarriesThrough verifies a "nie"-mode doc's Mode
// is passed through onto the VM regardless of the underlying Compose
// standing (a nie doc is never composed, so StandingOf reports "absent" —
// the template branches on vm.Mode == nie BEFORE looking at State to render
// "ausgeblendet (nie)" instead of nothing).
func TestBuildDocContext_NieModeCarriesThrough(t *testing.T) {
	st := usecase.ContextStanding{State: "absent"}
	vm := BuildDocContext(st, "backstage", domain.ContextModeNie)
	if vm == nil {
		t.Fatal("BuildDocContext must not return nil for a nie-mode doc")
	}
	if vm.Mode != domain.ContextModeNie {
		t.Errorf("Mode = %q, want nie", vm.Mode)
	}
}
