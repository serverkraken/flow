package webui

// Render tests for CockpitMain (cockpit_main.templ) — the cockpit's single
// scrolling content column: instr-band, Enthält, Wissen, Buchungen, Puls.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
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

// TestCockpitMain_PanelErrShown verifies that a non-empty d.PanelErr renders
// an inline error banner in #cockpit-main (Task 7: every #cockpit-main
// mutation handler — Nachbuchen, session edit/delete — sets PanelErr on its
// error path; without this banner the error would be invisible to the user).
func TestCockpitMain_PanelErrShown(t *testing.T) {
	d := seededCockpit()
	d.PanelErr = "ungültige Zeit"
	out := renderToBuf(t, context.Background(), CockpitMain(d))
	if !strings.Contains(out, "ungültige Zeit") {
		t.Fatalf("CockpitMain with PanelErr missing the error message:\n%s", out)
	}

	clean := seededCockpit()
	cleanOut := renderToBuf(t, context.Background(), CockpitMain(clean))
	if strings.Contains(cleanOut, `role="alert"`) {
		t.Fatalf("CockpitMain without PanelErr must not render an alert banner:\n%s", cleanOut)
	}
}

// TestCockpitBuchungRow_RunningOmitsDurationAndControls verifies a running
// Buchung row (Stop==nil) shows the running indicator instead of a fixed
// duration and renders NEITHER the edit link nor the delete trigger (carried
// over from the deleted cockpitSessionRow's equivalent assertion).
func TestCockpitBuchungRow_RunningOmitsDurationAndControls(t *testing.T) {
	row := CockpitSessionRow{ID: "s5", Date: "Mi 01.07.", Span: "10:00–…", Running: true}
	out := renderToBuf(t, context.Background(), cockpitBuchungRow("n1", row, false))
	if !strings.Contains(out, "10:00–…") {
		t.Errorf("running Buchung row missing the open span: %.400s", out)
	}
	if strings.Contains(out, "edit=s5") || strings.Contains(out, "delete-session-s5") {
		t.Errorf("running Buchung row must NOT show edit/delete controls: %.400s", out)
	}
}

// TestCockpitBuchungRow_NodePillShownForSubtreeHiddenForOwnOnly pins the
// containment node-pill (Spec §4): showPill=true renders the booked node's
// kind label + name as a linked .targetlink; showPill=false (a Repo's
// own-only list) renders neither, even though the row carries the same
// node fields (carried over from the deleted cockpitSessionNodePill test).
func TestCockpitBuchungRow_NodePillShownForSubtreeHiddenForOwnOnly(t *testing.T) {
	row := CockpitSessionRow{
		ID: "s3", Date: "Di 30.06.", Span: "09:00–10:00", Dur: "1:00 h",
		NodeID: "r1", NodeName: "flow-api", NodeKind: domain.KindRepo,
	}
	shown := renderToBuf(t, context.Background(), cockpitBuchungRow("e1", row, true))
	if !strings.Contains(shown, "flow-api") || !strings.Contains(shown, `href="/nodes/r1"`) {
		t.Errorf("showPill=true: node pill missing name/link: %.400s", shown)
	}

	hidden := renderToBuf(t, context.Background(), cockpitBuchungRow("r1", row, false))
	if strings.Contains(hidden, "flow-api") {
		t.Errorf("showPill=false: node pill must NOT render: %.400s", hidden)
	}
}

// TestCockpitBuchungRow_EditDeleteControlsForCompletedSession verifies that a
// completed row carries the Edit round-trip link (hx-get ?edit={sid} against
// /main, landing back in #cockpit-main) and the Delete confirm-dialog
// trigger (carried over from the deleted cockpitSessionRow test, updated for
// the /main fragment route).
func TestCockpitBuchungRow_EditDeleteControlsForCompletedSession(t *testing.T) {
	row := CockpitSessionRow{ID: "s4", Date: "Mi 01.07.", Span: "08:00–09:00", Dur: "1:00 h"}
	out := renderToBuf(t, context.Background(), cockpitBuchungRow("n1", row, false))

	if !strings.Contains(out, `hx-get="/nodes/n1/main?edit=s4"`) {
		t.Errorf("completed row missing edit round-trip link: %.500s", out)
	}
	if !strings.Contains(out, `hx-target="#cockpit-main"`) {
		t.Errorf("completed row edit link must target #cockpit-main: %.500s", out)
	}
	if !strings.Contains(out, `data-dialog-open="delete-session-s4"`) {
		t.Errorf("completed row missing delete confirm trigger: %.500s", out)
	}
	if !strings.Contains(out, `hx-post="/nodes/n1/sessions/s4/delete"`) {
		t.Errorf("completed row delete confirm missing hx-post to sessions/s4/delete: %.500s", out)
	}
}

// TestCockpitMain_WissenCapShowsAllLink pins the section cap contract
// (Gate-Finding: 187 ungedeckelte Zeilen): when more docs exist than rows
// rendered, the sect-h carries an in-place "Alle N ›" expander targeting
// #cockpit-main with ?wissen=all; the expanded view (WissenAll) drops it.
func TestCockpitMain_WissenCapShowsAllLink(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.WissenRows = []WissenRow{{ID: "d1", Title: "Doc 1", ChipClass: "tc-b", ChipLabel: "Projekt"}}
	d.WissenTotal = 187
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitMain(d))
	for _, want := range []string{"Alle 187 ›", "wissen=all", `hx-target="#cockpit-main"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("capped wissen section misses %q:\n%s", want, out)
		}
	}
	d.WissenAll = true
	out = renderToBuf(t, ctx, CockpitMain(d))
	if strings.Contains(out, "Alle 187 ›") {
		t.Fatalf("expanded wissen section must not repeat the all-link:\n%s", out)
	}
}
