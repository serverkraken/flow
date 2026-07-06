package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// TestCockpitRail_ChainAndBindings verifies the Meta-Spalte's two .rail .blk
// panels: the Kette (this -> ancestors -> inherited rate) and Bindings. The
// "Kontext für Agenten" curation block is explicitly L5 scope and must NOT
// appear here yet.
func TestCockpitRail_ChainAndBindings(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Ancestors = []domain.Node{
		{ID: "n1", Name: "backstage", Kind: domain.KindRepo},
		{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement},
	}
	d.ChainStats = map[string]domain.NodeRollup{
		"n1": {Total: 2*time.Hour + 41*time.Minute},
		"e1": {Total: 304*time.Hour + 46*time.Minute},
	}
	d.Rate = "87,50 €/h"
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitRailBlocks(d))
	// Kette-Block: echte Inhalte — beide Ketten-Ebenen + geerbter Satz (KEINE tote Assertion):
	for _, want := range []string{`class="blk"`, "Kette", `class="krow"`, "RTL Extern", "87,50", "304:46"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rail Kette misses %q:\n%s", want, out)
		}
	}
	// Bindings-Block vorhanden (leer → Empty-Label „Keine Bindings"):
	if !strings.Contains(out, "Bindings") {
		t.Fatalf("rail must show Bindings block:\n%s", out)
	}
	// Kontext-Block ist L5 → darf NICHT hier sein:
	if strings.Contains(out, "Kontext für Agenten") || strings.Contains(out, "Kuratieren") {
		t.Fatalf("Kontext block is L5, must not appear in L2 rail")
	}
}

// TestCockpitRail_ContributorsBlockOnlyWhenPresent pins the Beiträger block
// (Task 7): it renders the subtree's distinct actors when railContributors
// filled d.Contributors, and stays absent entirely when empty (no orphan
// heading for a quiet subtree).
func TestCockpitRail_ContributorsBlockOnlyWhenPresent(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Contributors = []string{"claude-code", "msoent"}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitRailBlocks(d))
	for _, want := range []string{"Beiträger", "claude-code", "msoent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rail Beiträger block misses %q:\n%s", want, out)
		}
	}

	empty := seededCockpit()
	empty.N.Kind = domain.KindRepo
	emptyOut := renderToBuf(t, ctx, CockpitRailBlocks(empty))
	if strings.Contains(emptyOut, "Beiträger") {
		t.Fatalf("rail must NOT render an empty Beiträger block: %s", emptyOut)
	}
}
