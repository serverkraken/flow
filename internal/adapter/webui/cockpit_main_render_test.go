package webui

// Render tests for CockpitMain (cockpit_main.templ) — the cockpit's single
// scrolling content column: instr-band, Enthält, Wissen, Buchungen, Puls.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestCockpitMain_WissenRowHasTypechipAndReadtime pins the Wissen section's
// row markup — type chip, title, reading time — and that the whole main
// column stays glyph-free and tab-free (Lesesaal, not Kristall).
func TestCockpitMain_WissenRowHasTypechipAndReadtime(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.WissenRows = []WissenRow{{ID: "d1", Title: "Token-Integration", ChipClass: "tc-b", ChipLabel: "Projekt", Meta: "Claude · heute", ReadTime: "18 min"}}
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	for _, want := range []string{"typechip", "tc-b", "Token-Integration", "18 min", "livechip"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cockpit main misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "pill-tab") {
		t.Fatalf("no glyphs / no tab strip in Lesesaal cockpit main")
	}
}

// TestCockpitMain_NonLeafShowsEnthaelt verifies the Enthält section lists a
// non-leaf node's direct children (Spec §5.3 "Kinder als Listen im Inhalt")
// with short name + rollup hours.
func TestCockpitMain_NonLeafShowsEnthaelt(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindVorhaben
	d.Children = []NodeChild{{N: domain.Node{ID: "c1", Name: "a/b/backstage", Kind: domain.KindRepo}, Total: "2:41 h"}}
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	if !strings.Contains(out, ">backstage<") || !strings.Contains(out, "2:41 h") {
		t.Fatalf("Enthält section must list children:\n%s", out)
	}
}

// TestCockpitMain_LeafHidesEnthaelt verifies a Repo (leaf for containment
// purposes — branch is reserved, no behavior yet) does NOT render the
// Enthält section at all.
func TestCockpitMain_LeafHidesEnthaelt(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	if strings.Contains(out, "cockpit.enthaelt.title") {
		t.Fatalf("i18n key leaked untranslated:\n%s", out)
	}
	if strings.Contains(out, `href="/nodes/new?parent=`) {
		t.Fatalf("Repo (leaf) must not render the Enthält add-child link:\n%s", out)
	}
}

// TestCockpitMain_EmptyStates verifies every section renders its own quiet
// empty line (no 0-tiles) when its data slice is empty.
func TestCockpitMain_EmptyStates(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindVorhaben
	d.SessionRows = nil
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	for _, want := range []string{
		"Keine Unterknoten",         // cockpit.struktur.empty
		"Noch keine Dokumente",      // cockpit.wissen.empty
		"Noch keine Buchungen",      // cockpit.worktime.empty
		"Noch keine Aktivität",      // activity.empty
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cockpit main missing empty-state text %q:\n%s", want, out)
		}
	}
}

// TestCockpitMain_WissenScopeToggleOnlyForNonRepo verifies the Wissen
// section's subtree/self scope toggle renders for Engagement/Vorhaben but
// not for a Repo (which has no subtree to toggle).
func TestCockpitMain_WissenScopeToggleOnlyForNonRepo(t *testing.T) {
	nonRepo := seededCockpit()
	nonRepo.N.Kind = domain.KindVorhaben
	out := renderToBuf(t, context.Background(), CockpitMain(nonRepo))
	if !strings.Contains(out, "scope=self") || !strings.Contains(out, "scope=subtree") {
		t.Errorf("non-Repo cockpit main missing Wissen scope toggle:\n%s", out)
	}

	repo := seededCockpit()
	repo.N.Kind = domain.KindRepo
	repoOut := renderToBuf(t, context.Background(), CockpitMain(repo))
	if strings.Contains(repoOut, "scope=self") {
		t.Errorf("Repo cockpit main must NOT render the Wissen scope toggle:\n%s", repoOut)
	}
}

// TestCockpitMain_BuchungenRendersSessionRow verifies the Buchungen section
// lists d.SessionRows as flat Lesesaal rows (date/span/tag/duration).
func TestCockpitMain_BuchungenRendersSessionRow(t *testing.T) {
	d := seededCockpit() // seeded with one SessionRow (Sa 28.06., 14:00–16:00, slice6, 2:00 h)
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	for _, want := range []string{"Sa 28.06.", "14:00–16:00", "slice6", "2:00 h"} {
		if !strings.Contains(out, want) {
			t.Errorf("Buchungen section missing %q:\n%s", want, out)
		}
	}
}
