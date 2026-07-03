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

// TestCockpitLayout_TwoColumns verifies the Direction-B two-column skeleton:
// cockpitBody renders #cockpit-rail (SSE-triggered on session/node events,
// BEFORE #cockpit-main in markup order) and #cockpit-main, wrapped in the
// lg:grid-cols-[340px_1fr] grid, plus the ONE shared session dialog mounted
// once in the page skeleton.
func TestCockpitLayout_TwoColumns(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, cockpitBody(d))

	if !strings.Contains(body, `id="cockpit-rail"`) {
		t.Errorf("cockpitBody missing #cockpit-rail div: %.400s", body)
	}
	if !strings.Contains(body, "sse:session.started") || !strings.Contains(body, "sse:node.updated") {
		t.Errorf("cockpitBody rail hx-trigger missing sse:session.started/sse:node.updated: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-main"`) {
		t.Errorf("cockpitBody missing #cockpit-main div: %.400s", body)
	}
	if railIdx, mainIdx := strings.Index(body, `id="cockpit-rail"`), strings.Index(body, `id="cockpit-main"`); railIdx < 0 || mainIdx < 0 || railIdx > mainIdx {
		t.Errorf("rail must appear BEFORE main in markup order: railIdx=%d mainIdx=%d body=%.400s", railIdx, mainIdx, body)
	}
	if !strings.Contains(body, "lg:grid-cols-[340px_1fr]") {
		t.Errorf("cockpitBody wrapper missing lg:grid-cols-[340px_1fr]: %.400s", body)
	}
	// The single shared session dialog (T3) is mounted once, add-mode, scoped
	// to this node's Nachbuchen endpoint.
	if !strings.Contains(body, `id="session-dialog"`) {
		t.Errorf("cockpitBody missing mounted #session-dialog: %.800s", body)
	}
	if !strings.Contains(body, `hx-post="/nodes/n1/sessions"`) {
		t.Errorf("session-dialog missing add-mode action /nodes/n1/sessions: %.800s", body)
	}
}

// TestCockpitRail_IdentityHero pins the hero box's render priority — uploaded
// logo (tile-contained when LogoShape=="tile", hex-cropped otherwise) > icon
// > glyph — and its fixed 110px dimensions. It also carries over the
// Logo>Icon>Glyph priority assertions from the deleted TestCockpitHex_* tests
// (cockpitHex itself is gone; the identity logic now lives in the rail).
func TestCockpitRail_IdentityHero(t *testing.T) {
	ctx := context.Background()

	tile := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", LogoRef: "abc123"}, LogoShape: "tile"}))
	if !strings.Contains(tile, "<img") || !strings.Contains(tile, "object-contain") {
		t.Errorf("tile logo must render <img> with object-contain: %.400s", tile)
	}
	if strings.Contains(tile, "clip-path") {
		t.Errorf("tile logo must NOT use clip-path: %.400s", tile)
	}
	if !strings.Contains(tile, "h-[110px] w-[110px]") {
		t.Errorf("hero box must be h-[110px] w-[110px] (tile): %.400s", tile)
	}

	hex := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", LogoRef: "abc123"}, LogoShape: "hex"}))
	if !strings.Contains(hex, "clip-path") {
		t.Errorf("hex logo must render with a clip-path style: %.400s", hex)
	}
	if !strings.Contains(hex, "h-[110px] w-[110px]") {
		t.Errorf("hero box must be h-[110px] w-[110px] (hex): %.400s", hex)
	}

	icon := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", Icon: "rocket", Color: "cyan"}}))
	if !strings.Contains(icon, "<svg") {
		t.Errorf("no-logo-with-icon must render inline <svg>: %.400s", icon)
	}
	if !strings.Contains(icon, "h-[110px] w-[110px]") {
		t.Errorf("hero box must be h-[110px] w-[110px] (icon): %.400s", icon)
	}

	// Carried over from the deleted TestCockpitHex_LogoIconGlyphPriority:
	// logo suppresses icon+glyph; icon suppresses glyph; glyph is the last resort.
	logoWins := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", LogoRef: "abc123def456", Icon: "rocket", Glyph: "◈", Color: "cyan"}, LogoShape: "hex"}))
	if !strings.Contains(logoWins, "/nodes/n1/logo?v=abc123def456") {
		t.Errorf("logo-bearing node must render the <img> URL, got: %s", logoWins)
	}
	if strings.Contains(logoWins, "<svg") || strings.Contains(logoWins, "◈") {
		t.Error("logo must suppress icon and glyph")
	}

	iconWins := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", Icon: "rocket", Glyph: "◈", Color: "cyan"}}))
	if !strings.Contains(iconWins, "<svg") {
		t.Errorf("icon-bearing node must render inline SVG, got: %s", iconWins)
	}
	if strings.Contains(iconWins, "◈") {
		t.Error("icon must suppress the glyph")
	}

	glyph := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", Glyph: "◈", Color: "cyan"}}))
	if !strings.Contains(glyph, "◈") {
		t.Errorf("fallback must render the glyph, got: %s", glyph)
	}

	// Carried over from the deleted TestCockpitHex_RendersGlyphAndClass:
	// an empty glyph falls back to the default identity glyph ◆.
	glyphDefault := renderToBuf(t, ctx, cockpitRailHero(NodeCockpit{N: domain.Node{ID: "n1", Color: "blue"}}))
	if !strings.Contains(glyphDefault, "◆") {
		t.Errorf("empty glyph must fall back to the default ◆, got: %s", glyphDefault)
	}
}

// TestCockpitRail_TimerStates pins the rail's per-state timer card markup —
// NodeTimer's state machine (cockpit_vm.go) is untouched, only its rendering
// moved here from the deleted NodeHead. It also consolidates the five old
// standalone TestCockpitTimer_* tests (Idle/Here/OtherBound/NotBookable/
// Unbound render assertions), now exercised through the full CockpitRail.
func TestCockpitRail_TimerStates(t *testing.T) {
	ctx := context.Background()

	here := seededCockpit()
	here.Timer = CockpitTimer{State: TimerHere, RunningID: "sess-1", RunningBase: 3600}
	hereBody := renderToBuf(t, ctx, CockpitRail(here))
	if !strings.Contains(hereBody, `/nodes/n1/stop`) {
		t.Errorf("TimerHere missing stop form action: %.600s", hereBody)
	}
	if !strings.Contains(hereBody, "data-timer") {
		t.Errorf("TimerHere missing data-timer live clock element: %.600s", hereBody)
	}

	idle := seededCockpit() // Timer.State = TimerIdle by default
	idle.TodayHere = "3:47 h"
	idle.CountsWork = true
	idleBody := renderToBuf(t, ctx, CockpitRail(idle))
	if !strings.Contains(idleBody, `/nodes/n1/start`) {
		t.Errorf("TimerIdle missing start form action: %.600s", idleBody)
	}
	if !strings.Contains(idleBody, "cta-glow") {
		t.Errorf("TimerIdle missing the cta-glow start button: %.600s", idleBody)
	}
	if !strings.Contains(idleBody, "3:47 h") {
		t.Errorf("TimerIdle missing the TodayHere value: %.600s", idleBody)
	}
	if !strings.Contains(idleBody, "zählt als Work") {
		t.Errorf("TimerIdle with CountsWork=true missing the Work word: %.600s", idleBody)
	}

	idlePrivat := seededCockpit()
	idlePrivat.CountsWork = false
	idlePrivatBody := renderToBuf(t, ctx, CockpitRail(idlePrivat))
	if !strings.Contains(idlePrivatBody, "Privat") {
		t.Errorf("TimerIdle with CountsWork=false missing the Privat word: %.600s", idlePrivatBody)
	}

	otherBound := seededCockpit()
	otherBound.Timer = CockpitTimer{State: TimerOtherBound, RunningID: "sess-2", OtherID: "n2", OtherName: "other-node"}
	otherBody := renderToBuf(t, ctx, CockpitRail(otherBound))
	if !strings.Contains(otherBody, "other-node") {
		t.Errorf("TimerOtherBound missing OtherName: %.600s", otherBody)
	}
	if !strings.Contains(otherBody, `/nodes/n1/switch`) {
		t.Errorf("TimerOtherBound missing switch form action: %.600s", otherBody)
	}
	if !strings.Contains(otherBody, "Wechseln") {
		t.Errorf("TimerOtherBound missing the Wechseln switch label: %.600s", otherBody)
	}

	notBookable := seededCockpit()
	notBookable.Timer = CockpitTimer{State: TimerNotBookable}
	nbBody := renderToBuf(t, ctx, CockpitRail(notBookable))
	if strings.Contains(nbBody, `hx-post="/nodes/n1/start"`) || strings.Contains(nbBody, `hx-post="/nodes/n1/stop"`) {
		t.Errorf("TimerNotBookable must render no start/stop form: %.600s", nbBody)
	}
	if !strings.Contains(nbBody, "nicht buchbar") {
		t.Errorf("TimerNotBookable missing hint text: %.600s", nbBody)
	}

	// Carried over from the deleted TestCockpitTimer_UnboundRendersHomeLink:
	// an unbooked running session shows a home link so the user can navigate
	// to Home to stop it.
	unbound := seededCockpit()
	unbound.Timer = CockpitTimer{State: TimerUnbound, RunningID: "sess-3"}
	unboundBody := renderToBuf(t, ctx, CockpitRail(unbound))
	if !strings.Contains(unboundBody, `href="/"`) {
		t.Errorf("TimerUnbound missing home link href=/: %.600s", unboundBody)
	}
}

// TestCockpitTabs_UebersichtDefault verifies NormalizeTab defaults to
// "uebersicht" and that the tab nav renders all 5 pill-tab links with the
// active marker and a count chip when TabCounts[key] > 0.
func TestCockpitTabs_UebersichtDefault(t *testing.T) {
	if got := NormalizeTab(""); got != "uebersicht" {
		t.Errorf(`NormalizeTab("")=%q want "uebersicht"`, got)
	}

	ctx := context.Background()
	d := seededCockpit()
	d.ActiveTab = "uebersicht"
	d.TabCounts = map[string]int{"wissen": 3, "struktur": 0, "bindings": 0}
	body := renderToBuf(t, ctx, CockpitTabsAndPanel(d))

	for _, tab := range []string{"uebersicht", "worktime", "wissen", "struktur", "bindings"} {
		url := `/nodes/n1/tab/` + tab
		if !strings.Contains(body, url) {
			t.Errorf("tab nav missing link %s: %.800s", url, body)
		}
	}
	// "pill-tab cursor-pointer" (not the bare "pill-tab" prefix, which also
	// matches the nav wrapper's own "pill-tabs" class) counts just the links.
	if got := strings.Count(body, `pill-tab cursor-pointer`); got != 5 {
		t.Errorf("expected 5 pill-tab links, got %d: %.800s", got, body)
	}
	if !strings.Contains(body, `aria-current="page"`) {
		t.Errorf("active Übersicht tab missing aria-current: %.600s", body)
	}
	if !strings.Contains(body, `<span class="pill-cnt">3</span>`) {
		t.Errorf("wissen tab count chip missing for TabCounts[wissen]=3: %.800s", body)
	}
}

// TestCockpitTabsAndPanel_TabLinksAndPanel verifies that CockpitTabsAndPanel renders
// a tab link for each of the 5 tabs (using the node ID in the URL), the #cockpit-panel
// container, and the SSE reload targeting #cockpit-main (not #cockpit-panel).
func TestCockpitTabsAndPanel_TabLinksAndPanel(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, CockpitTabsAndPanel(d))

	for _, tab := range []string{"uebersicht", "worktime", "wissen", "struktur", "bindings"} {
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
// aria-current="page" and the correct URL, while an inactive tab does not,
// and that the count chip (pill-cnt) renders only when count > 0.
func TestCockpitTabLink_ActiveMarkup(t *testing.T) {
	ctx := context.Background()

	active := renderToBuf(t, ctx, cockpitTabLink("n1", "worktime", "cockpit.tab.worktime", 0, true))
	if !strings.Contains(active, `aria-current="page"`) {
		t.Errorf("active tab missing aria-current: %.300s", active)
	}
	if !strings.Contains(active, `/nodes/n1/tab/worktime`) {
		t.Errorf("active tab missing URL: %.300s", active)
	}

	inactive := renderToBuf(t, ctx, cockpitTabLink("n1", "wissen", "cockpit.tab.wissen", 0, false))
	if strings.Contains(inactive, `aria-current`) {
		t.Errorf("inactive tab must NOT have aria-current: %.300s", inactive)
	}
	if !strings.Contains(inactive, `/nodes/n1/tab/wissen`) {
		t.Errorf("inactive tab missing URL: %.300s", inactive)
	}
	if strings.Contains(inactive, "pill-cnt") {
		t.Errorf("tab link with count=0 must NOT render a pill-cnt chip: %.300s", inactive)
	}

	withCount := renderToBuf(t, ctx, cockpitTabLink("n1", "wissen", "cockpit.tab.wissen", 5, false))
	if !strings.Contains(withCount, `<span class="pill-cnt">5</span>`) {
		t.Errorf("tab link with count=5 missing pill-cnt chip: %.300s", withCount)
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

// TestCockpitPanel_UebersichtRendersPlaceholder verifies that the "uebersicht"
// tab (the new default landing) renders a placeholder EmptyState — the real
// Übersicht feed (rollup tiles, Work/Privat split, composition/chain, pulse,
// knowledge) is a later task; this only pins that the tab is wired end-to-end
// and shows a translated, non-empty state (no raw "…" loading stub).
func TestCockpitPanel_UebersichtRendersPlaceholder(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.ActiveTab = "uebersicht"
	body := renderToBuf(t, ctx, cockpitPanel(d))

	if !strings.Contains(body, "Übersicht") {
		t.Errorf("uebersicht placeholder missing translated title: %.500s", body)
	}
	if strings.Contains(body, "…") {
		t.Errorf("uebersicht placeholder must not use a bare ellipsis loading stub: %.500s", body)
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
// cockpit-rail and cockpit-main structural IDs that cockpitBody emits).
func TestNodeCockpitShell_IncludesNodeContent(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, nodeCockpitShell(d))

	// nodeCockpitShell wraps AppShell around nodeBreadcrumb + cockpitBody;
	// cockpitBody emits #cockpit-rail and #cockpit-main.
	if !strings.Contains(body, `id="cockpit-rail"`) {
		t.Errorf("nodeCockpitShell missing #cockpit-rail: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-main"`) {
		t.Errorf("nodeCockpitShell missing #cockpit-main: %.400s", body)
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

// TestCockpitRail_RendersNameAndCard verifies that CockpitRail renders the
// node name and the identity card's rounded-3xl glass wrapper — carried over
// from the deleted NodeHead (whose section this replaces).
func TestCockpitRail_RendersNameAndCard(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, CockpitRail(d))

	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("CockpitRail missing node name 'flow-rebuild': %.400s", body)
	}
	if !strings.Contains(body, "rounded-3xl") {
		t.Errorf("CockpitRail missing rounded-3xl card class: %.400s", body)
	}
}

// TestCockpitRail_WithRateLabel verifies that a non-empty Rate field renders the
// inherited rate label in the identity card's id-meta box.
func TestCockpitRail_WithRateLabel(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.Rate = "120 EUR/h"
	body := renderToBuf(t, ctx, CockpitRail(d))

	if !strings.Contains(body, "120 EUR/h") {
		t.Errorf("CockpitRail with Rate missing rate label '120 EUR/h': %.500s", body)
	}
	if !strings.Contains(body, "flow-rebuild") {
		t.Errorf("CockpitRail with Rate missing node name: %.500s", body)
	}
}

// TestCockpitRail_WithDescription verifies that a non-empty DescriptionHTML is
// rendered inside the identity card (below the badges).
func TestCockpitRail_WithDescription(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	d.DescriptionHTML = "<p>A detailed description of the node.</p>"
	body := renderToBuf(t, ctx, CockpitRail(d))

	if !strings.Contains(body, "A detailed description of the node.") {
		t.Errorf("CockpitRail with DescriptionHTML missing description content: %.600s", body)
	}
	// The prose wrapper must be present.
	if !strings.Contains(body, `class="prose`) {
		t.Errorf("CockpitRail with DescriptionHTML missing prose wrapper class: %.600s", body)
	}
}

// TestCockpitRail_WithoutDescription verifies that when DescriptionHTML is empty
// no empty prose block is emitted (no orphan wrapper div).
func TestCockpitRail_WithoutDescription(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit() // DescriptionHTML is zero value = ""
	body := renderToBuf(t, ctx, CockpitRail(d))

	if strings.Contains(body, `class="prose`) {
		t.Errorf("CockpitRail without DescriptionHTML must NOT render a prose block: %.600s", body)
	}
}

// TestCockpitRail_RendersEditLink pins the only UI entry point to the node edit
// form (name/color/icon/logo/rate): the rail must link /nodes/{id}/edit as a
// full-page navigation (hx-boost off, canonical htmx rule) — carried over from
// the deleted NodeHead (commit c8c4dee) into its identity card badge row.
func TestCockpitRail_RendersEditLink(t *testing.T) {
	body := renderToBuf(t, context.Background(), CockpitRail(seededCockpit()))
	if !strings.Contains(body, `href="/nodes/n1/edit"`) {
		t.Errorf("cockpit rail must link the edit form, got: %s", body)
	}
	if !strings.Contains(body, `hx-boost="false"`) {
		t.Error("edit link must opt out of hx-boost (full-page form)")
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
	if !strings.Contains(icon, "rgb(var(--cyan))") {
		t.Errorf("icon must be tinted in the node color (token-reactive), got: %s", icon)
	}
	glyph := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(glyph, "◆") {
		t.Errorf("glyph fallback broken, got: %s", glyph)
	}
}

// TestCockpitIdMeta_InheritedRateSource pins that the "geerbt von" row names the
// NEAREST rate-setting ancestor (matching domain.ResolveRate), not the root: a
// repo inheriting a rate set on an intermediate Vorhaben must attribute it to
// the Vorhaben, not the top Engagement.
func TestCockpitIdMeta_InheritedRateSource(t *testing.T) {
	ctx := context.Background()
	rate := &domain.Money{Amount: 5000, Currency: "EUR"}
	d := NodeCockpit{
		N:    domain.Node{ID: "repo1", Name: "flow", Kind: domain.KindRepo}, // no own rate
		Rate: "50,00 €/h",
		// leaf→root: Vorhaben carries the rate, Engagement (root) does not.
		Ancestors: []domain.Node{
			{ID: "vor1", Name: "Plattform-Umbau", Kind: domain.KindVorhaben, Rate: rate},
			{ID: "eng1", Name: "Kundenarbeit", Kind: domain.KindEngagement},
		},
	}
	out := renderToBuf(t, ctx, cockpitIdMeta(d))
	if !strings.Contains(out, "Plattform-Umbau") {
		t.Errorf("inherited-rate row must name the nearest rate ancestor (Vorhaben): %s", out)
	}
	if strings.Contains(out, "Kundenarbeit") {
		t.Errorf("inherited-rate row must NOT name the root when an intermediate sets the rate: %s", out)
	}

	// Pure-helper direct check incl. the "no ancestor sets a rate" case.
	if got := cockpitRateSource(d); got != "Plattform-Umbau" {
		t.Errorf("cockpitRateSource = %q, want Plattform-Umbau", got)
	}
	none := NodeCockpit{N: domain.Node{ID: "x"}, Ancestors: []domain.Node{{ID: "y", Name: "Y"}}}
	if got := cockpitRateSource(none); got != "" {
		t.Errorf("cockpitRateSource with no rate ancestor = %q, want empty", got)
	}
}
