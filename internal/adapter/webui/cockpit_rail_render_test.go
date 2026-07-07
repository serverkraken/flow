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
// "Kontext für Agenten" panel is guarded on d.Context != nil (Task 5); this
// fixture leaves Context nil (seededCockpit()'s default), so it must NOT
// appear here — see TestCockpitRail_ContextPanel_PresentWhenSet for the
// populated case.
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
	// Kontext-Block: d.Context ist hier nil → darf nicht rendern:
	if strings.Contains(out, "Kontext für Agenten") || strings.Contains(out, "Kuratieren") {
		t.Fatalf("nil Context must render no panel:\n%s", out)
	}
}

// TestCockpitRail_ContextPanel_PresentWhenSet verifies the L5 "Kontext für
// Agenten" instrument panel (meter + Enthalten/Verworfen/Angepinnt + numbered
// pins + Kuratieren-Link) renders when d.Context is populated, using the real
// values that make the meter "full" (Pct >= 95) so the warn-notice path is
// exercised too — not just presence.
func TestCockpitRail_ContextPanel_PresentWhenSet(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.N.ID = "n1"
	d.Context = &CockpitContextVM{
		NodeID: "n1", UsedStr: "11.891", CapStr: "12.000", Pct: 99, Full: true,
		IncludedN: 24, DroppedN: 65, PinnedN: 3,
		TopPins: []ContextPinVM{
			{Num: "01", Title: "Tailwind v4 + templ Gotchas"},
			{Num: "02", Title: "Plans need a main-wiring task"},
			{Num: "03", Title: "Keine Monolithen"},
		},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitRailBlocks(d))
	for _, want := range []string{
		"Kontext für Agenten", `class="meter full"`, "11.891", "12.000",
		"fast voll", "24 Docs", "65", "Enthalten", "Verworfen (Budget)", "Angepinnt",
		"Tailwind v4 + templ Gotchas", "Plans need a main-wiring task", "Keine Monolithen",
		"Kuratieren", "/kontext/n1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("context panel misses %q:\n%s", want, out)
		}
	}
}

// TestCockpitRail_ContextPanel_AbsentWhenNil confirms the guard: a nil
// d.Context (unwired ComposeContext, or a failed compose) renders no panel at
// all rather than crashing — seededCockpit()'s default leaves Context nil.
func TestCockpitRail_ContextPanel_AbsentWhenNil(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitRailBlocks(d))
	if strings.Contains(out, "Kontext für Agenten") || strings.Contains(out, "Kuratieren") {
		t.Fatalf("nil Context must render no panel:\n%s", out)
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
