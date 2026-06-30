package webui

import (
	"bytes"
	"context"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// renderComp is a shared helper that renders a templ.Component to a
// bytes.Buffer (not *runtime.Buffer), which triggers the !IsBuffer defer
// blocks in generated template code and gets those statements covered.
func renderComp(t *testing.T, ctx context.Context, comp templ.Component) {
	t.Helper()
	var buf bytes.Buffer
	if err := comp.Render(ctx, &buf); err != nil {
		t.Errorf("Render error: %v", err)
	}
}

// TestHeuteIsBuffer_Coverage exercises heute_templ.go private functions via
// direct calls with a bytes.Buffer to cover !IsBuffer defer blocks.
func TestHeuteIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()

	// heuteHero dereferences vm.Running so supply a non-nil session.
	heroVM := HeuteVM{Running: &domain.WorkSession{ID: "s1"}}

	cases := []struct {
		name string
		comp templ.Component
	}{
		{"worktimeSubnav", worktimeSubnav("heute")},
		{"heuteBody", heuteBody(HeuteVM{})},
		{"heuteOuter", heuteOuter(HeuteVM{})},
		{"heuteHero", heuteHero(heroVM)},
		{"heuteStat", heuteStat("worktime.today", "8h", "blue")},
		{"heuteStartCard", heuteStartCard(HeuteVM{})},
		{"heuteWeekCard", heuteWeekCard(HeuteVM{})},
		{"heuteSessionsCard", heuteSessionsCard(HeuteVM{})},
		{"heuteSessionRow", heuteSessionRow(HeuteVM{}, components.SessionRowVM{})},
		{"heuteNachbuchenDialog", heuteNachbuchenDialog(HeuteVM{})},
		{"heuteAddForm", heuteAddForm(HeuteVM{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestHomeIsBuffer_Coverage exercises home_templ.go private functions.
// homeHero dereferences vm.Running.ID unconditionally so a non-nil session is required.
func TestHomeIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()

	// homeHero unconditionally accesses vm.Running.ID
	heroVM := HomeVM{Running: &domain.WorkSession{ID: "h1"}}

	cases := []struct {
		name string
		comp templ.Component
	}{
		{"homeBody", homeBody(HomeVM{})},
		{"homeOuter", homeOuter(HomeVM{})},
		{"homeHero", homeHero(heroVM)},
		{"homeStartCard", homeStartCard(HomeVM{})},
		{"homeLogstreamInner", homeLogstreamInner(HomeVM{})},
		{"homeNewestDocRow", homeNewestDocRow(DocRow{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestWocheIsBuffer_Coverage exercises woche_templ.go private functions.
func TestWocheIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		comp templ.Component
	}{
		{"wocheBody", wocheBody(WocheVM{})},
		{"wocheOuter", wocheOuter(WocheVM{})},
		{"wocheNav", wocheNav(WocheVM{})},
		{"wocheDaysCard", wocheDaysCard(WocheVM{})},
		{"wocheDayRow", wocheDayRow(WocheDayVM{})},
		{"wocheDayBar", wocheDayBar(WocheDayVM{})},
		{"wocheWeekendRow", wocheWeekendRow(WocheDayVM{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestFreiIsBuffer_Coverage exercises frei_templ.go private functions.
func TestFreiIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		comp templ.Component
	}{
		{"freiBody", freiBody(FreiVM{})},
		{"freiOuter", freiOuter(FreiVM{})},
		{"freiAddCard", freiAddCard(FreiVM{})},
		{"freiListCard", freiListCard(FreiVM{})},
		{"freiRow", freiRow(FreiRowVM{})},
		{"freiSettingsCard", freiSettingsCard(FreiVM{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestCockpitIsBuffer_Coverage exercises cockpit_templ.go private functions.
func TestCockpitIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		comp templ.Component
	}{
		{"nodeCockpitShell", nodeCockpitShell(NodeCockpit{})},
		{"nodeBreadcrumb", nodeBreadcrumb(NodeCockpit{})},
		{"cockpitBody", cockpitBody(NodeCockpit{})},
		{"cockpitTimer", cockpitTimer(NodeCockpit{})},
		{"cockpitRollup", cockpitRollup(NodeCockpit{})},
		{"cockpitTile", cockpitTile("cockpit.total", "42h", "12h", false)},
		{"cockpitHex", cockpitHex(domain.Node{})},
		{"cockpitTabLink", cockpitTabLink("sessions", "sessions", "cockpit.sessions", false)},
		{"cockpitPanel", cockpitPanel(NodeCockpit{})},
		{"cockpitSessionRow", cockpitSessionRow(CockpitSessionRow{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestNodesIsBuffer_Coverage exercises nodes_templ.go private functions.
// nodeFormBody and nodeFormInner check editing != nil before dereferencing,
// so passing nil is safe.
func TestNodesIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		comp templ.Component
	}{
		{"nodesBody", nodesBody(NodesPageData{})},
		{"nodesOuter", nodesOuter(NodesPageData{})},
		{"nodeTreeRow", nodeTreeRow(TreeRow{})},
		{"nodeKindBadge", nodeKindBadge(domain.NodeKind(""))},
		{"nodeGlyphSwatch", nodeGlyphSwatch(domain.Node{})},
		{"nodeStatusBadge", nodeStatusBadge(domain.NodeStatus(""))},
		{"nodeFormBody nil", nodeFormBody(NodeFormData{}, nil)},
		{"nodeFormInner nil", nodeFormInner(NodeFormData{}, nil)},
		{"nodeFormBody non-nil", nodeFormBody(NodeFormData{}, &domain.Node{})},
		{"nodeFormInner non-nil", nodeFormInner(NodeFormData{}, &domain.Node{})},
		{"nodeKindOption", nodeKindOption("engagement", "node.kind.engagement", "")},
		{"nodeStatusOption", nodeStatusOption("active", "node.status.active", "")},
		{"nodeColorRadio", nodeColorRadio("color", "#3b82f6")},
		{"nodeGlyphRadio", nodeGlyphRadio("\u25b6", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}

// TestHistorieIsBuffer_Coverage exercises historie_templ.go private functions.
func TestHistorieIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		comp templ.Component
	}{
		{"historieCalBody", historieCalBody(HistorieVM{})},
		{"historieCalOuter", historieCalOuter(HistorieVM{})},
		{"historieHeading", historieHeading(HistorieVM{})},
		{"historieToolbar", historieToolbar(HistorieVM{})},
		{"historieWeek", historieWeek(HistorieVM{})},
		{"historieDayHead", historieDayHead(HistorieDayVM{})},
		{"historieTimeAxis", historieTimeAxis(HistorieVM{})},
		{"historieDayColumn", historieDayColumn(HistorieVM{}, HistorieDayVM{})},
		{"historieBlock", historieBlock("2024-01-15", "2024-01-15", components.SessionBlockVM{})},
		{"historieAgendaRow", historieAgendaRow("2024-01-15", "2024-01-15", components.SessionBlockVM{}, components.SessionRowVM{})},
		{"historieAgenda", historieAgenda(HistorieVM{})},
		{"historieMonth", historieMonth(HistorieVM{})},
		{"historieMonthCell", historieMonthCell(HistorieMonthCellVM{})},
		{"historieLegend", historieLegend()},
		{"historieListBody", historieListBody(HistorieListVM{})},
		{"historieListOuter", historieListOuter(HistorieListVM{})},
		{"historieListHeading", historieListHeading()},
		{"HistorieSelectionBarC", HistorieSelectionBarC(nil, "cal")},
		{"historieEditDialog", historieEditDialog(nil)},
		{"historieEditForm", historieEditForm(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderComp(t, ctx, tc.comp)
		})
	}
}
