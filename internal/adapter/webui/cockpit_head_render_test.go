package webui

// Cockpit head (spine) + instr-band render tests — package webui (same-package)
// per the established cockpit_render_test.go convention (renderToBuf + a
// bytes.Buffer, exercising the !IsBuffer defer path in generated templ code).
//
// Ctx: context.Background() resolves to the German catalog (i18n.Default =
// DE) — matching the existing TestCockpitRail_TimerStates convention — so
// German text assertions below need no explicit i18n.WithLocale.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestCockpitHead_SpineShowsShortNameAndFullPath(t *testing.T) {
	d := seededCockpit()
	d.N.Name = "gitlab.com/dataalliance/products/foolu/product/backstage"
	d.N.Kind = domain.KindRepo
	out := renderToBuf(t, context.Background(), CockpitHead(d))
	if !strings.Contains(out, "<h1") || !strings.Contains(out, ">backstage<") {
		t.Fatalf("spine must show ShortName as h1:\n%s", out)
	}
	if !strings.Contains(out, `class="fullpath"`) || !strings.Contains(out, d.N.Name) {
		t.Fatalf("spine must show full path in .fullpath (no truncation):\n%s", out)
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "▲") {
		t.Fatalf("no kind glyphs in Lesesaal spine")
	}
	if !strings.Contains(out, "av-96") {
		t.Fatalf("spine misses 96er identity:\n%s", out)
	}
	// instr-Band gehört NICHT in den Spine (liegt in CockpitMain):
	if strings.Contains(out, `class="instr"`) {
		t.Fatalf("instr band must live in CockpitMain, not the spine")
	}
}

// TestCockpitHead_UpCrumbsExcludeSelf verifies SpineCrumbs derives the "up"
// chain from d.Ancestors without the trailing self segment (self renders as
// the <h1>, not as a crumb link).
func TestCockpitHead_UpCrumbsExcludeSelf(t *testing.T) {
	d := seededCockpit()
	d.N.ID = "n3"
	d.N.Name = "backstage"
	d.N.Kind = domain.KindRepo
	d.Ancestors = []domain.Node{
		{ID: "n3", Name: "backstage"},
		{ID: "n2", Name: "backstage · Vorhaben"},
		{ID: "n1", Name: "RTL Extern"},
	}
	crumbs := SpineCrumbs(d)
	if len(crumbs) != 2 {
		t.Fatalf("SpineCrumbs must exclude self, got %d: %+v", len(crumbs), crumbs)
	}
	if crumbs[0].Label != "RTL Extern" || crumbs[1].Label != "backstage · Vorhaben" {
		t.Fatalf("SpineCrumbs must be root→leaf: %+v", crumbs)
	}

	out := renderToBuf(t, context.Background(), CockpitHead(d))
	if !strings.Contains(out, ">RTL Extern<") || !strings.Contains(out, ">backstage · Vorhaben<") {
		t.Fatalf("spine .up must render the ancestor crumbs:\n%s", out)
	}
	if strings.Contains(out, `href="/nodes/n3"`) {
		t.Fatalf("spine .up must not link to self:\n%s", out)
	}
}

// TestCockpitHead_MetaLineShowsStatusEditAndLogo verifies the spine-meta row:
// neutral kind chip, status word (i18n, not StatusBadge's non-token colors),
// edit link, and the logo-add link with its hint title.
func TestCockpitHead_MetaLineShowsStatusEditAndLogo(t *testing.T) {
	d := seededCockpit()
	d.N.Status = domain.NodePaused
	out := renderToBuf(t, context.Background(), CockpitHead(d))
	if !strings.Contains(out, `class="k"`) {
		t.Fatalf("spine-meta missing the neutral kind chip: %s", out)
	}
	if !strings.Contains(out, "pausiert") {
		t.Fatalf("spine-meta missing the status word: %s", out)
	}
	if !strings.Contains(out, "Bearbeiten") {
		t.Fatalf("spine-meta missing the edit link: %s", out)
	}
	// Full-page navigation (hx-boost off) — the only UI entry point to the node
	// edit form, carried over from the deleted rail's edit-link regression pin
	// (commit c8c4dee).
	if !strings.Contains(out, `href="/nodes/n1/edit"`) {
		t.Fatalf("spine edit link must point at /nodes/n1/edit: %s", out)
	}
	if !strings.Contains(out, `hx-boost="false"`) {
		t.Fatalf("spine edit link must opt out of hx-boost (full-page form): %s", out)
	}
	if !strings.Contains(out, "Logo hinzufügen") {
		t.Fatalf("spine-meta missing the logo-add link: %s", out)
	}
	if !strings.Contains(out, `data-copy="`+d.N.Name+`"`) {
		t.Fatalf(".fullpath copy button must carry the full name: %s", out)
	}
}

func TestCockpitInstr_RunningShowsStop(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Timer = CockpitTimer{State: TimerHere, RunningBase: 754}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	for _, want := range []string{`class="instr"`, `class="stats"`, "/stop", "data-timer", `hx-target="#cockpit-main"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("running instr misses %q:\n%s", want, out)
		}
	}
}

func TestCockpitInstr_IdleShowsStart(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Timer = CockpitTimer{State: TimerIdle}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(out, "/start") || !strings.Contains(out, `hx-target="#cockpit-main"`) {
		t.Fatalf("idle instr missing start form: %s", out)
	}
	if strings.Contains(out, "/stop") {
		t.Fatalf("idle instr must not render a stop form: %s", out)
	}
}

// TestCockpitInstr_OtherBoundTruncatesLongPath carries forward the L1 lesson
// (fix 5d012f4): a long remote name running elsewhere must show ShortName as
// link text with the full path only in title, never as unbroken link text.
func TestCockpitInstr_OtherBoundTruncatesLongPath(t *testing.T) {
	d := seededCockpit()
	longPath := "gitlab.com/dataalliance/infra/common/cmdb"
	d.Timer = CockpitTimer{State: TimerOtherBound, OtherID: "n2", OtherName: longPath}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(out, ">cmdb<") {
		t.Fatalf("OtherBound must show ShortName as link text: %s", out)
	}
	if strings.Contains(out, ">"+longPath+"<") {
		t.Fatalf("OtherBound must not render the full path as unbroken link text: %s", out)
	}
	if !strings.Contains(out, `title="`+longPath+`"`) {
		t.Fatalf("OtherBound must carry the full path in title: %s", out)
	}
}

func TestCockpitInstr_UnboundShowsHomeLink(t *testing.T) {
	d := seededCockpit()
	d.Timer = CockpitTimer{State: TimerUnbound}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(out, `href="/"`) {
		t.Fatalf("unbound instr missing home link: %s", out)
	}
}

// TestCockpitInstr_NotBookableHidesActions verifies a non-bookable node (e.g.
// Branch) renders neither timer forms nor the "Nachbuchen" quick action.
func TestCockpitInstr_NotBookableHidesActions(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindBranch
	d.Timer = CockpitTimer{State: TimerNotBookable}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(out, "nicht buchbar") {
		t.Fatalf("not-bookable instr missing hint text: %s", out)
	}
	if strings.Contains(out, "/start") || strings.Contains(out, "/stop") || strings.Contains(out, "/switch") {
		t.Fatalf("not-bookable instr must render no timer form: %s", out)
	}
	if strings.Contains(out, "data-dialog-open=\"session-dialog\"") {
		t.Fatalf("not-bookable instr must hide the Nachbuchen button: %s", out)
	}
}

// TestCockpitInstr_ZeroStatsShowDash pins "Nullen ohne Bühne" (Spec §4): a
// repo with no time yet renders "—" instead of "0:00 h" in the stats line.
func TestCockpitInstr_ZeroStatsShowDash(t *testing.T) {
	d := seededCockpit()
	d.TodayHere = "0:00 h"
	d.Rollup.Total = 0
	d.Timer = CockpitTimer{State: TimerIdle}
	out := renderToBuf(t, context.Background(), cockpitInstr(d))
	if strings.Contains(out, "0:00 h") {
		t.Fatalf("zero stats must render as \"—\", not \"0:00 h\":\n%s", out)
	}
	if !strings.Contains(out, "—") {
		t.Fatalf("zero stats missing the dash placeholder:\n%s", out)
	}
}

// TestCockpitInstr_ChainSegmentUsesRootNameWhenWired verifies the third stats
// segment shows the wired root-engagement name+total once Task 7 populates
// ChainRootName/ChainRootTotal, falling back to the generic "Kette" label
// until then.
func TestCockpitInstr_ChainSegmentUsesRootNameWhenWired(t *testing.T) {
	d := seededCockpit()
	d.Timer = CockpitTimer{State: TimerIdle}
	unwired := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(unwired, "Kette") {
		t.Fatalf("unwired chain segment must fall back to the generic label:\n%s", unwired)
	}

	d.ChainRootName = "RTL Extern"
	d.ChainRootTotal = "304:46 h"
	wired := renderToBuf(t, context.Background(), cockpitInstr(d))
	if !strings.Contains(wired, "RTL Extern") || !strings.Contains(wired, "304:46 h") {
		t.Fatalf("wired chain segment must show the root name+total:\n%s", wired)
	}
}
