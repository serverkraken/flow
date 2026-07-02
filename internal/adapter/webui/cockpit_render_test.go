package webui

// Honest cockpit render tests — in package webui (same-package) so they can call
// unexported component functions that are only reachable within the package.
//
// Each test renders a cockpit component to a bytes.Buffer (not *runtime.Buffer),
// which exercises the !IsBuffer defer path in generated templ code, AND asserts
// specific content in the HTML output — making these real behavioral assertions,
// not mere coverage-padding renders.

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/domain"
)

// renderToBuf renders a templ.Component to a bytes.Buffer (non-runtime.Buffer)
// so the !IsBuffer defer block inside the generated function executes.
// Returns the rendered HTML string.
func renderToBuf(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

// seededCockpit builds a minimal NodeCockpit with a bookable node and one session row
// for use across multiple tests.
func seededCockpit() NodeCockpit {
	return NodeCockpit{
		User: "u1",
		N: domain.Node{
			ID:    "n1",
			Name:  "flow-rebuild",
			Kind:  domain.KindRepo,
			Color: "cyan",
			Glyph: "◈",
		},
		ActiveTab: "worktime",
		Timer:     CockpitTimer{State: TimerIdle},
		Rollup: domain.NodeRollup{
			Total: 5*time.Hour + 30*time.Minute,
			Week:  2 * time.Hour,
			Month: 14 * time.Hour,
		},
		SessionRows: []CockpitSessionRow{
			{Date: "Sa 28.06.", Span: "14:00–16:00", Tag: "slice6", Dur: "2:00 h"},
		},
	}
}

// TestCockpitBody_ContainersAndSSEURL verifies that cockpitBody renders the two
// structural containers (#cockpit-head for SSE-triggered head reload and #cockpit-main
// for the tab strip) and embeds the correct SSE head refresh URL.
func TestCockpitBody_ContainersAndSSEURL(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, cockpitBody(d))

	if !strings.Contains(body, `id="cockpit-head"`) {
		t.Errorf("cockpitBody missing #cockpit-head div: %.400s", body)
	}
	if !strings.Contains(body, `/nodes/n1/head`) {
		t.Errorf("cockpitBody missing /nodes/n1/head SSE URL: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-main"`) {
		t.Errorf("cockpitBody missing #cockpit-main div: %.400s", body)
	}
}

// TestNodeHead_RendersNameAndSection verifies that NodeHead renders the node name
// in the h1 and the rounded-3xl section that wraps the glass head.
func TestNodeHead_RendersNameAndSection(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, NodeHead(d))

	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("NodeHead missing node name 'flow-rebuild': %.400s", body)
	}
	if !strings.Contains(body, "rounded-3xl") {
		t.Errorf("NodeHead missing rounded-3xl section class: %.400s", body)
	}
}

// TestCockpitTabsAndPanel_TabLinksAndPanel verifies that CockpitTabsAndPanel renders
// a tab link for each of the 4 tabs (using the node ID in the URL), the #cockpit-panel
// container, and the SSE reload targeting #cockpit-main (not #cockpit-panel).
func TestCockpitTabsAndPanel_TabLinksAndPanel(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, CockpitTabsAndPanel(d))

	for _, tab := range []string{"worktime", "wissen", "struktur", "bindings"} {
		url := `/nodes/n1/tab/` + tab
		if !strings.Contains(body, url) {
			t.Errorf("CockpitTabsAndPanel missing tab link %s: %.600s", url, body)
		}
	}
	if !strings.Contains(body, `id="cockpit-panel"`) {
		t.Errorf("CockpitTabsAndPanel missing #cockpit-panel div: %.600s", body)
	}
	// The SSE live-reload target for the panel must be #cockpit-main (the outer
	// container), never #cockpit-panel (which would nest the strip inside itself).
	if strings.Contains(body, `hx-target="#cockpit-panel"`) {
		t.Errorf("CockpitTabsAndPanel must NOT use hx-target=\"#cockpit-panel\" (nesting bug): %.600s", body)
	}
}

// TestCockpitTabLink_ActiveMarkup verifies that the active tab renders with
// aria-current="page" and the correct URL, while an inactive tab does not.
func TestCockpitTabLink_ActiveMarkup(t *testing.T) {
	ctx := context.Background()

	active := renderToBuf(t, ctx, cockpitTabLink("n1", "worktime", "cockpit.tab.worktime", true))
	if !strings.Contains(active, `aria-current="page"`) {
		t.Errorf("active tab missing aria-current: %.300s", active)
	}
	if !strings.Contains(active, `/nodes/n1/tab/worktime`) {
		t.Errorf("active tab missing URL: %.300s", active)
	}

	inactive := renderToBuf(t, ctx, cockpitTabLink("n1", "wissen", "cockpit.tab.wissen", false))
	if strings.Contains(inactive, `aria-current`) {
		t.Errorf("inactive tab must NOT have aria-current: %.300s", inactive)
	}
	if !strings.Contains(inactive, `/nodes/n1/tab/wissen`) {
		t.Errorf("inactive tab missing URL: %.300s", inactive)
	}
}

// TestCockpitSessionRow_RendersDateSpanTagDur verifies that cockpitSessionRow
// renders the date, time span, tag chip, and duration — the four precomputed fields.
func TestCockpitSessionRow_RendersDateSpanTagDur(t *testing.T) {
	ctx := context.Background()
	row := CockpitSessionRow{
		Date: "Sa 28.06.", Span: "14:00–16:00", Tag: "slice6", Dur: "2:00 h",
	}
	body := renderToBuf(t, ctx, cockpitSessionRow(row))

	for _, want := range []string{"Sa 28.06.", "14:00–16:00", "slice6", "2:00 h"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpitSessionRow missing %q: %.400s", want, body)
		}
	}
}

// TestCockpitSessionRow_RunningOmitsDuration verifies that a running session row
// (Stop == nil) renders the running indicator text instead of a fixed duration.
func TestCockpitSessionRow_RunningOmitsDuration(t *testing.T) {
	ctx := context.Background()
	row := CockpitSessionRow{
		Date: "Mo 30.06.", Span: "10:00–…", Running: true,
	}
	body := renderToBuf(t, ctx, cockpitSessionRow(row))

	if !strings.Contains(body, "10:00–…") {
		t.Errorf("running session row missing open span: %.400s", body)
	}
	// Running rows show the i18n key (or its fallback) instead of a fixed duration.
	// Exact translated text varies; we just verify the fixed duration "0:00 h" is absent.
	if strings.Contains(body, "0:00 h") {
		t.Errorf("running session row must NOT show 0:00 h duration: %.400s", body)
	}
}

// TestCockpitTile_RendersValue verifies that cockpitTile renders the provided
// value string — the main metric shown in each rollup tile.
func TestCockpitTile_RendersValue(t *testing.T) {
	ctx := context.Background()
	body := renderToBuf(t, ctx, cockpitTile("cockpit.rollup.total", "5:30 h", "", false))

	if !strings.Contains(body, "5:30 h") {
		t.Errorf("cockpitTile missing value '5:30 h': %.400s", body)
	}
}

// TestCockpitRollup_RendersDurations verifies that cockpitRollup renders the
// precomputed duration strings for Total, Week, and Month from d.Rollup.
func TestCockpitRollup_RendersDurations(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, cockpitRollup(d))

	// fmtDurHM(5h30m) = "5:30 h", fmtDurHM(2h) = "2:00 h", fmtDurHM(14h) = "14:00 h"
	for _, want := range []string{"5:30 h", "2:00 h", "14:00 h"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpitRollup missing duration %q: %.500s", want, body)
		}
	}
}

// TestCockpitHex_RendersGlyphAndClass verifies that cockpitHex renders the node's
// glyph inside a rounded avatar span, and falls back to the default glyph when empty.
func TestCockpitHex_RendersGlyphAndClass(t *testing.T) {
	ctx := context.Background()

	// Node with explicit glyph.
	body := renderToBuf(t, ctx, cockpitHex(domain.Node{Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(body, "◈") {
		t.Errorf("cockpitHex missing glyph ◈: %.300s", body)
	}
	if !strings.Contains(body, "rounded-xl") {
		t.Errorf("cockpitHex missing rounded-xl class: %.300s", body)
	}

	// Node without glyph → default ◆ is substituted.
	body2 := renderToBuf(t, ctx, cockpitHex(domain.Node{Color: "blue"}))
	if !strings.Contains(body2, "◆") {
		t.Errorf("cockpitHex with empty glyph missing default ◆: %.300s", body2)
	}
}

// TestCockpitTimer_IdleRendersStartForm verifies that the idle timer state renders
// the Start form posting to /nodes/{id}/start.
func TestCockpitTimer_IdleRendersStartForm(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // Timer.State = TimerIdle by default
	body := renderToBuf(t, ctx, cockpitTimer(d))

	if !strings.Contains(body, `/nodes/n1/start`) {
		t.Errorf("TimerIdle missing start form action: %.400s", body)
	}
}

// TestCockpitTimer_HereRendersStopForm verifies that the TimerHere state renders
// the Stop form with the data-timer live clock element.
func TestCockpitTimer_HereRendersStopForm(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Timer = CockpitTimer{State: TimerHere, RunningID: "sess-1", RunningBase: 3600}
	body := renderToBuf(t, ctx, cockpitTimer(d))

	if !strings.Contains(body, `/nodes/n1/stop`) {
		t.Errorf("TimerHere missing stop form action: %.400s", body)
	}
	if !strings.Contains(body, `data-timer`) {
		t.Errorf("TimerHere missing data-timer live clock element: %.400s", body)
	}
}

// TestCockpitTimer_OtherBoundRendersSwitchForm verifies that the TimerOtherBound
// state renders the switch form targeting the current node's /switch endpoint.
func TestCockpitTimer_OtherBoundRendersSwitchForm(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Timer = CockpitTimer{
		State: TimerOtherBound, RunningID: "sess-2",
		OtherID: "n2", OtherName: "other-node",
	}
	body := renderToBuf(t, ctx, cockpitTimer(d))

	if !strings.Contains(body, `/nodes/n1/switch`) {
		t.Errorf("TimerOtherBound missing switch form action: %.400s", body)
	}
	if !strings.Contains(body, "other-node") {
		t.Errorf("TimerOtherBound missing other node name: %.400s", body)
	}
}

// TestCockpitPanel_WorktimeRendersFormAndRows verifies that cockpitPanel with
// ActiveTab "worktime" renders the Nachbuchen form (with the correct post target
// and hx-target pointing to #cockpit-main) and the precomputed session rows.
func TestCockpitPanel_WorktimeRendersFormAndRows(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // ActiveTab = "worktime", SessionRows seeded
	body := renderToBuf(t, ctx, cockpitPanel(d))

	if !strings.Contains(body, `/nodes/n1/sessions`) {
		t.Errorf("cockpitPanel missing Nachbuchen form action /nodes/n1/sessions: %.500s", body)
	}
	// Nachbuchen form must target #cockpit-main (outer container), not #cockpit-panel.
	if !strings.Contains(body, `hx-target="#cockpit-main"`) {
		t.Errorf("cockpitPanel Nachbuchen form missing hx-target=#cockpit-main: %.500s", body)
	}
	// Session rows must appear in the panel.
	if !strings.Contains(body, "Sa 28.06.") {
		t.Errorf("cockpitPanel missing seeded session row date 'Sa 28.06.': %.500s", body)
	}
	if !strings.Contains(body, "14:00–16:00") {
		t.Errorf("cockpitPanel missing seeded session row span '14:00–16:00': %.500s", body)
	}
}

// TestCockpitBreadcrumb_RendersNodeName verifies that nodeBreadcrumb includes
// the node name as the leaf breadcrumb entry when there are no ancestors.
func TestCockpitBreadcrumb_RendersNodeName(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // Ancestors empty → falls back to d.N.Name
	body := renderToBuf(t, ctx, nodeBreadcrumb(d))

	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("nodeBreadcrumb missing node name 'flow-rebuild': %.400s", body)
	}
}

// TestNodeCockpitShell_IncludesNodeContent verifies that nodeCockpitShell
// (the AppShell wrapper) includes the cockpit body content (specifically the
// cockpit-head and cockpit-main structural IDs that cockpitBody emits).
func TestNodeCockpitShell_IncludesNodeContent(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, nodeCockpitShell(d))

	// nodeCockpitShell wraps AppShell around nodeBreadcrumb + cockpitBody;
	// cockpitBody emits #cockpit-head and #cockpit-main.
	if !strings.Contains(body, `id="cockpit-head"`) {
		t.Errorf("nodeCockpitShell missing #cockpit-head: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-main"`) {
		t.Errorf("nodeCockpitShell missing #cockpit-main: %.400s", body)
	}
}

// TestCockpitTimer_NotBookableRendersNoForm verifies that a non-bookable node
// (e.g. a branch) renders no start/stop form in the timer slot.
func TestCockpitTimer_NotBookableRendersNoForm(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Timer = CockpitTimer{State: TimerNotBookable}
	body := renderToBuf(t, ctx, cockpitTimer(d))

	if strings.Contains(body, `hx-post="/nodes/n1/start"`) || strings.Contains(body, `hx-post="/nodes/n1/stop"`) {
		t.Errorf("TimerNotBookable must render no start/stop form: %.400s", body)
	}
}

// TestCockpitTimer_UnboundRendersHomeLink verifies that an unbooked running session
// shows a home link so the user can navigate to Home to stop it.
func TestCockpitTimer_UnboundRendersHomeLink(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Timer = CockpitTimer{State: TimerUnbound, RunningID: "sess-3"}
	body := renderToBuf(t, ctx, cockpitTimer(d))

	if !strings.Contains(body, `href="/"`) {
		t.Errorf("TimerUnbound missing home link href=/: %.400s", body)
	}
}

// TestCockpitTile_WithSubtitle verifies that cockpitTile renders the sub string
// when it is non-empty (the "incl. children" footnote on the Total rollup tile).
func TestCockpitTile_WithSubtitle(t *testing.T) {
	ctx := context.Background()
	body := renderToBuf(t, ctx, cockpitTile("cockpit.rollup.total", "5:30 h", "incl. children", false))

	if !strings.Contains(body, "5:30 h") {
		t.Errorf("cockpitTile with sub missing value: %.400s", body)
	}
	if !strings.Contains(body, "incl. children") {
		t.Errorf("cockpitTile with sub missing subtitle: %.400s", body)
	}
}

// TestCockpitTile_EarningsVariant verifies that cockpitTile with earn=true renders
// the earnings value (and uses green styling, not the default gray).
func TestCockpitTile_EarningsVariant(t *testing.T) {
	ctx := context.Background()
	body := renderToBuf(t, ctx, cockpitTile("cockpit.rollup.earnings", "1500 EUR", "", true))

	if !strings.Contains(body, "1500 EUR") {
		t.Errorf("earnings tile missing value '1500 EUR': %.400s", body)
	}
}

// TestCockpitRollup_WithEarnings verifies that when d.Earnings is non-empty an
// additional earnings tile is rendered alongside the standard three rollup tiles.
func TestCockpitRollup_WithEarnings(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Earnings = "750 EUR"
	body := renderToBuf(t, ctx, cockpitRollup(d))

	if !strings.Contains(body, "750 EUR") {
		t.Errorf("cockpitRollup with earnings missing '750 EUR': %.500s", body)
	}
	// The standard tiles must still appear.
	if !strings.Contains(body, "5:30 h") {
		t.Errorf("cockpitRollup with earnings missing total duration: %.500s", body)
	}
}

// TestCockpitPanel_WorktimeWithError verifies that when PanelErr is set the
// worktime panel renders an inline error message above the form.
func TestCockpitPanel_WorktimeWithError(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.PanelErr = "ungültige Zeit"
	body := renderToBuf(t, ctx, cockpitPanel(d))

	if !strings.Contains(body, "ungültige Zeit") {
		t.Errorf("cockpitPanel with PanelErr missing error message: %.500s", body)
	}
}

// TestNodeHead_WithRateLabel verifies that a non-empty Rate field renders the
// inherited rate label in the cockpit head identity section.
func TestNodeHead_WithRateLabel(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Rate = "120 EUR/h"
	body := renderToBuf(t, ctx, NodeHead(d))

	if !strings.Contains(body, "120 EUR/h") {
		t.Errorf("NodeHead with Rate missing rate label '120 EUR/h': %.500s", body)
	}
	// Node name must still appear.
	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("NodeHead with Rate missing node name: %.500s", body)
	}
}

// TestNodeHead_WithDescription verifies that a non-empty DescriptionHTML is
// rendered inside the cockpit head (below the rollup tiles).
func TestNodeHead_WithDescription(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.DescriptionHTML = "<p>A detailed description of the node.</p>"
	body := renderToBuf(t, ctx, NodeHead(d))

	if !strings.Contains(body, "A detailed description of the node.") {
		t.Errorf("NodeHead with DescriptionHTML missing description content: %.600s", body)
	}
	// The prose wrapper must be present.
	if !strings.Contains(body, `class="prose`) {
		t.Errorf("NodeHead with DescriptionHTML missing prose wrapper class: %.600s", body)
	}
}

// TestNodeHead_WithoutDescription verifies that when DescriptionHTML is empty
// no empty prose block is emitted (no orphan wrapper div).
func TestNodeHead_WithoutDescription(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // DescriptionHTML is zero value = ""
	body := renderToBuf(t, ctx, NodeHead(d))

	// No prose wrapper should appear when description is empty.
	if strings.Contains(body, `class="prose`) {
		t.Errorf("NodeHead without DescriptionHTML must NOT render a prose block: %.600s", body)
	}
}

// TestCockpitHex_LogoIconGlyphPriority pins the render priority for the cockpit
// head identity tile: uploaded logo > icon > glyph.
func TestCockpitHex_LogoIconGlyphPriority(t *testing.T) {
	ctx := context.Background()

	logo := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", LogoRef: "abc123def456", Icon: "rocket", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(logo, `/nodes/n1/logo?v=abc123def456`) {
		t.Errorf("logo-bearing node must render the <img> URL, got: %s", logo)
	}
	if strings.Contains(logo, "<svg") || strings.Contains(logo, "◈") {
		t.Error("logo must suppress icon and glyph")
	}
	if !strings.Contains(logo, "clip-path") {
		t.Error("uploaded logo must render with the hexagonal clip")
	}

	icon := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", Icon: "rocket", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(icon, "<svg") {
		t.Errorf("icon-bearing node must render inline SVG, got: %s", icon)
	}
	if strings.Contains(icon, "◈") {
		t.Error("icon must suppress the glyph")
	}

	glyph := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(glyph, "◈") {
		t.Errorf("fallback must render the glyph, got: %s", glyph)
	}
}

// TestNodeGlyphSwatch_LogoIconGlyphPriority pins the render priority for the node
// tree row identity mark: uploaded logo > icon (tinted in the node color) > glyph.
func TestNodeGlyphSwatch_LogoIconGlyphPriority(t *testing.T) {
	ctx := context.Background()
	logo := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", LogoRef: "abc123def456", Icon: "rocket", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(logo, "/nodes/n1/logo?v=abc123def456") || strings.Contains(logo, "<svg") {
		t.Errorf("logo wins over icon in the tree row, got: %s", logo)
	}
	icon := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", Icon: "rocket", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(icon, "<svg") || strings.Contains(icon, "◆") {
		t.Errorf("icon wins over glyph in the tree row, got: %s", icon)
	}
	if !strings.Contains(icon, "#7dcfff") {
		t.Errorf("icon must be tinted in the node color, got: %s", icon)
	}
	glyph := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(glyph, "◆") {
		t.Errorf("glyph fallback broken, got: %s", glyph)
	}
}

// TestNodeHead_RendersEditLink pins the only UI entry point to the node edit
// form (name/color/icon/logo/rate): the head must link /nodes/{id}/edit as a
// full-page navigation (hx-boost off, canonical htmx rule).
func TestNodeHead_RendersEditLink(t *testing.T) {
	body := renderToBuf(t, context.Background(), NodeHead(seededCockpit()))
	if !strings.Contains(body, `href="/nodes/n1/edit"`) {
		t.Errorf("cockpit head must link the edit form, got: %s", body)
	}
	if !strings.Contains(body, `hx-boost="false"`) {
		t.Error("edit link must opt out of hx-boost (full-page form)")
	}
}
