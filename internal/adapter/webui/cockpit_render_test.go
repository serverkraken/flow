package webui

// Honest cockpit render tests — in package webui (same-package) so they can call
// unexported component functions that are only reachable within the package.
//
// Each test renders a cockpit component to a bytes.Buffer (not *runtime.Buffer),
// which exercises the !IsBuffer defer path in generated templ code, AND asserts
// specific content in the HTML output — making these real behavioral assertions,
// not mere coverage-padding renders.
//
// cockpit.templ itself is now just the page skeleton (NodeView/cockpitBody):
// three independent SSE fragments around CockpitHead (T4), CockpitMain (T6),
// and CockpitRailBlocks (T5) — no tabs (Task 7 Flatten). Per-component render
// tests for the fragments live in their own files (cockpit_head_render_test.go,
// cockpit_main_render_test.go, cockpit_rail_render_test.go).

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
		Timer: CockpitTimer{State: TimerIdle},
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

// TestCockpitBody_ThreeSSEFragments verifies the flat layout (Task 7): three
// independent SSE containers in markup order head→main→rail, each hx-get-ing
// its own fragment route and hx-trigger-ing the mutation table's SSE events,
// wrapped by the .cock grid around main+rail. The shared add-mode session
// dialog is mounted once, scoped to this node's Nachbuchen endpoint.
func TestCockpitBody_ThreeSSEFragments(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, cockpitBody(d))

	if !strings.Contains(body, `id="cockpit-head"`) {
		t.Errorf("cockpitBody missing #cockpit-head: %.400s", body)
	}
	if !strings.Contains(body, `hx-get="/nodes/n1/head"`) || !strings.Contains(body, "sse:node.updated, sse:node.moved") {
		t.Errorf("cockpitBody head fragment missing hx-get/hx-trigger: %.400s", body)
	}
	if !strings.Contains(body, `class="cock"`) {
		t.Errorf("cockpitBody missing .cock grid: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-main"`) {
		t.Errorf("cockpitBody missing #cockpit-main: %.400s", body)
	}
	if !strings.Contains(body, `hx-get="/nodes/n1/main"`) || !strings.Contains(body, "sse:activity.logged") {
		t.Errorf("cockpitBody main fragment missing hx-get/hx-trigger: %.400s", body)
	}
	if !strings.Contains(body, `id="cockpit-rail"`) || !strings.Contains(body, `class="rail"`) {
		t.Errorf("cockpitBody missing #cockpit-rail aside: %.400s", body)
	}
	if !strings.Contains(body, `hx-get="/nodes/n1/rail"`) {
		t.Errorf("cockpitBody rail fragment missing hx-get: %.400s", body)
	}

	headIdx := strings.Index(body, `id="cockpit-head"`)
	mainIdx := strings.Index(body, `id="cockpit-main"`)
	railIdx := strings.Index(body, `id="cockpit-rail"`)
	if headIdx < 0 || mainIdx < 0 || railIdx < 0 || headIdx >= mainIdx || mainIdx >= railIdx {
		t.Errorf("fragments must appear head→main→rail in markup order: head=%d main=%d rail=%d", headIdx, mainIdx, railIdx)
	}

	if !strings.Contains(body, `id="session-dialog"`) {
		t.Errorf("cockpitBody missing mounted #session-dialog: %.800s", body)
	}
	if !strings.Contains(body, `hx-post="/nodes/n1/sessions"`) {
		t.Errorf("session-dialog missing add-mode action /nodes/n1/sessions: %.800s", body)
	}
	if !strings.Contains(body, "/static/js/clipboard.js") {
		t.Errorf("cockpitBody missing the clipboard.js copy-affordance script: %.800s", body)
	}
}

// TestCockpitBody_NoTabRemnants pins that the flatten removed every tab-strip
// trace (pill-tabs, /tab/ routes, the old CockpitTabsAndPanel wrapper).
func TestCockpitBody_NoTabRemnants(t *testing.T) {
	body := renderToBuf(t, context.Background(), cockpitBody(seededCockpit()))
	for _, gone := range []string{"pill-tabs", "pill-tab", "/tab/", "CockpitTabsAndPanel", "data-tabstrip"} {
		if strings.Contains(body, gone) {
			t.Errorf("tab remnant %q still present:\n%s", gone, body)
		}
	}
}

// TestCockpitBody_EditSessionDialogMountedWhenSet verifies the ?edit={sid}
// round-trip: when d.EditSession is set, cockpitBody mounts the shared
// SessionDialog pre-opened (native <dialog open>, no click needed) alongside
// the add-mode dialog, prefilled from the session.
func TestCockpitBody_EditSessionDialogMountedWhenSet(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	start := time.Date(2026, 6, 28, 14, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	d.EditSession = &domain.WorkSession{ID: "s1", Start: start, Stop: &stop, Note: "impl", Tags: []string{"slice6"}}
	body := renderToBuf(t, ctx, cockpitBody(d))

	if !strings.Contains(body, `id="session-dialog-edit"`) {
		t.Errorf("missing edit dialog element: %.700s", body)
	}
	if !strings.Contains(body, " open") {
		t.Errorf("edit dialog must render pre-opened (native <dialog open>): %.700s", body)
	}
	if !strings.Contains(body, `hx-post="/nodes/n1/sessions/s1/edit"`) {
		t.Errorf("edit dialog form must post to /nodes/n1/sessions/s1/edit: %.700s", body)
	}
	if !strings.Contains(body, "impl") {
		t.Errorf("edit dialog must prefill note: %.700s", body)
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
// (the AppShell wrapper) includes the cockpit body content — specifically the
// three SSE-fragment IDs that cockpitBody emits.
func TestNodeCockpitShell_IncludesNodeContent(t *testing.T) {
	ctx := context.Background()
	d := seededCockpit()
	body := renderToBuf(t, ctx, nodeCockpitShell(d))

	for _, id := range []string{`id="cockpit-head"`, `id="cockpit-main"`, `id="cockpit-rail"`} {
		if !strings.Contains(body, id) {
			t.Errorf("nodeCockpitShell missing %s: %.400s", id, body)
		}
	}
}

// TestCockpitRateSource_NearestAncestor pins that cockpitRateSource names the
// NEAREST rate-setting ancestor (matching domain.ResolveRate), not the root: a
// repo inheriting a rate set on an intermediate Vorhaben must attribute it to
// the Vorhaben, not the top Engagement. (The old cockpitIdMeta card that
// rendered this in the rail is gone — CockpitHead/CockpitRailBlocks (T4/T5)
// surface the resolved rate value itself; this pure helper is still real,
// tested logic.)
func TestCockpitRateSource_NearestAncestor(t *testing.T) {
	rate := &domain.Money{Amount: 5000, Currency: "EUR"}
	d := NodeCockpit{
		N: domain.Node{ID: "repo1", Name: "flow", Kind: domain.KindRepo}, // no own rate
		// leaf→root: Vorhaben carries the rate, Engagement (root) does not.
		Ancestors: []domain.Node{
			{ID: "vor1", Name: "Plattform-Umbau", Kind: domain.KindVorhaben, Rate: rate},
			{ID: "eng1", Name: "Kundenarbeit", Kind: domain.KindEngagement},
		},
	}
	if got := cockpitRateSource(d); got != "Plattform-Umbau" {
		t.Errorf("cockpitRateSource = %q, want Plattform-Umbau", got)
	}

	none := NodeCockpit{N: domain.Node{ID: "x"}, Ancestors: []domain.Node{{ID: "y", Name: "Y"}}}
	if got := cockpitRateSource(none); got != "" {
		t.Errorf("cockpitRateSource with no rate ancestor = %q, want empty", got)
	}
}
