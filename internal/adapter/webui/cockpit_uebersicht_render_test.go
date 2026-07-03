package webui

// Output-asserting render tests for the Übersicht feed (Step 6 of the task
// brief): tiles with delta, split widths, kind-differentiated Comp vs Chain,
// a pulse row with pill + AGENT tag, and the docs card.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestCockpitUebersicht_TilesWithDelta(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	vm := UebersichtVM{
		TotalStr: "10:00 h", WeekStr: "12:05 h", WeekDelta: "+2h 05m",
		MonthStr: "40:00 h", Earnings: "950.00 EUR",
	}
	body := renderToBuf(t, ctx, CockpitUebersicht(d, vm))

	for _, want := range []string{"10:00 h", "12:05 h", "+2h 05m", "40:00 h", "950.00 EUR"} {
		if !strings.Contains(body, want) {
			t.Errorf("tiles missing %q: %.800s", want, body)
		}
	}
}

func TestCockpitUebersicht_SplitWidths(t *testing.T) {
	ctx := context.Background()
	vm := UebersichtVM{HasSplit: true, WorkPct: 40, WorkWeekStr: "4:00 h", PrivatWeekStr: "6:00 h"}
	body := renderToBuf(t, ctx, uebersichtSplitCard(vm))

	if !strings.Contains(body, "width:40%") {
		t.Errorf("split card missing work width:40%%: %.600s", body)
	}
	if !strings.Contains(body, "width:60%") {
		t.Errorf("split card missing privat width:60%%: %.600s", body)
	}
	if !strings.Contains(body, "4:00 h") || !strings.Contains(body, "6:00 h") {
		t.Errorf("split card missing the Work/Privat totals: %.600s", body)
	}

	collapsed := renderToBuf(t, ctx, uebersichtSplitCard(UebersichtVM{HasSplit: false}))
	if strings.Contains(collapsed, "split-bar") {
		t.Errorf("collapsed split card must not render the bar: %.600s", collapsed)
	}
}

// TestCockpitUebersicht_RepoRendersChainNotComp pins the Containment rule
// (spec §4): a Repo cockpit shows the chain card and NOT the composition card.
func TestCockpitUebersicht_RepoRendersChainNotComp(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // N.Kind = domain.KindRepo
	vm := UebersichtVM{
		Kind: domain.KindRepo,
		Chain: []ChainRow{
			{Label: "flow-rebuild", Kind: domain.KindRepo, DurStr: "4:00 h", Pct: 8, This: true},
			{DurStr: "50:00 h", Pct: 100, Sum: true},
		},
	}
	body := renderToBuf(t, ctx, CockpitUebersicht(d, vm))

	if !strings.Contains(body, "Fließt nach oben") {
		t.Errorf("repo cockpit missing the chain card title: %.800s", body)
	}
	if strings.Contains(body, "Woraus besteht das?") {
		t.Errorf("repo cockpit must NOT render the composition card title: %.800s", body)
	}
	if !strings.Contains(body, "hier") {
		t.Errorf("repo cockpit missing the This-row 'hier' badge: %.800s", body)
	}
}

// TestCockpitUebersicht_EngagementRendersCompNotChain pins the other half of
// the Containment rule: Engagement/Vorhaben show composition, never chain.
func TestCockpitUebersicht_EngagementRendersCompNotChain(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.N.Kind = domain.KindEngagement
	vm := UebersichtVM{
		Kind: domain.KindEngagement,
		Comp: []CompRow{
			{ID: "c1", Name: "flow", Kind: domain.KindRepo, Color: "cyan", DurStr: "2:00 h", Pct: 50, Live: true, LastAct: "vor 3 Min"},
		},
	}
	body := renderToBuf(t, ctx, CockpitUebersicht(d, vm))

	if !strings.Contains(body, "Woraus besteht das?") {
		t.Errorf("engagement cockpit missing the composition card title: %.800s", body)
	}
	if strings.Contains(body, "Fließt nach oben") {
		t.Errorf("engagement cockpit must NOT render the chain card title: %.800s", body)
	}
	if !strings.Contains(body, "/nodes/c1") {
		t.Errorf("composition row missing the child link: %.800s", body)
	}
	if !strings.Contains(body, "animate-breathe") {
		t.Errorf("live composition row missing the breathing live dot: %.800s", body)
	}
	if !strings.Contains(body, "vor 3 Min") {
		t.Errorf("composition row missing LastAct: %.800s", body)
	}
}

// TestCockpitUebersicht_PulseRowWithAgentTag verifies an agent-kind pulse row
// renders the AGENT chip and the form-coded target pill.
func TestCockpitUebersicht_PulseRowWithAgentTag(t *testing.T) {
	ctx := context.Background()
	vm := UebersichtVM{
		Pulse: []ActivityRowVM{
			{
				ActorKind: "agent", ActorRef: "claude-code", VerbKey: "activity.verb.session.started",
				TargetName: "flow-rebuild", TargetKind: domain.KindRepo, TargetHref: "/nodes/n1",
				RelTime: "vor 2 Min",
			},
		},
	}
	body := renderToBuf(t, ctx, uebersichtPulseCard(vm))

	if !strings.Contains(body, "AI-AGENT") {
		t.Errorf("agent pulse row missing the AGENT chip: %.800s", body)
	}
	if !strings.Contains(body, "claude-code") {
		t.Errorf("pulse row missing the actor name: %.800s", body)
	}
	if !strings.Contains(body, "/nodes/n1") || !strings.Contains(body, "flow-rebuild") {
		t.Errorf("pulse row missing the target pill: %.800s", body)
	}
	if !strings.Contains(body, "vor 2 Min") {
		t.Errorf("pulse row missing RelTime: %.800s", body)
	}
}

// TestCockpitUebersicht_HumanPulseRowNoAgentTag pins the negative case: a
// human actor must NOT render the AGENT chip.
func TestCockpitUebersicht_HumanPulseRowNoAgentTag(t *testing.T) {
	ctx := context.Background()
	vm := UebersichtVM{
		Pulse: []ActivityRowVM{
			{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.started", RelTime: "vor 1 Min"},
		},
	}
	body := renderToBuf(t, ctx, uebersichtPulseCard(vm))
	if strings.Contains(body, "AI-AGENT") {
		t.Errorf("human pulse row must NOT render the AGENT chip: %.600s", body)
	}
}

// TestCockpitUebersicht_DocsCard verifies the docs card renders each doc's
// title + meta, and the "alle N ›" footer link targeting #cockpit-main when
// DocsTotal exceeds the 3 shown.
func TestCockpitUebersicht_DocsCard(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	vm := UebersichtVM{
		Docs: []UebersichtDoc{
			{ID: "d1", Title: "Architektur", Meta: "vor 1 Std"},
			{ID: "d2", Title: "Runbook", Meta: "vor 2 Std"},
		},
		DocsTotal: 5,
	}
	body := renderToBuf(t, ctx, uebersichtDocsCard(d, vm))

	for _, want := range []string{"Architektur", "vor 1 Std", "Runbook", "vor 2 Std", "/wissen/d1", "/wissen/d2"} {
		if !strings.Contains(body, want) {
			t.Errorf("docs card missing %q: %.800s", want, body)
		}
	}
	if !strings.Contains(body, `hx-get="/nodes/n1/tab/wissen"`) || !strings.Contains(body, `hx-target="#cockpit-main"`) {
		t.Errorf("docs card footer link must hx-get the wissen tab targeting #cockpit-main: %.800s", body)
	}
	if !strings.Contains(body, "alle 5") {
		t.Errorf("docs card footer missing the total count: %.800s", body)
	}
}

// TestCockpitUebersicht_DocsCardNoFooterWhenAllShown verifies the "alle N ›"
// footer link is absent when DocsTotal is <= 3 (nothing more to see).
func TestCockpitUebersicht_DocsCardNoFooterWhenAllShown(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	vm := UebersichtVM{
		Docs:      []UebersichtDoc{{ID: "d1", Title: "Architektur", Meta: "vor 1 Std"}},
		DocsTotal: 1,
	}
	body := renderToBuf(t, ctx, uebersichtDocsCard(d, vm))
	if strings.Contains(body, "alle 1") {
		t.Errorf("docs card must not show the footer link when DocsTotal<=3: %.600s", body)
	}
}
