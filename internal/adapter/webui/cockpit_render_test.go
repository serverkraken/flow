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

// testCtx is the bare context used by webui-package render tests — mirrors
// components_test's own testCtx (components/base_test.go); duplicated here
// because the two are separate packages and this one is unexported.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
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

// TestCockpitBody_FourSSEFragments ist entfallen: die flache Cockpit-SEITE
// gibt es nicht mehr. Ihre Nachfolge tritt
// httpserver.TestNodeEntry_ShellAndUnknownNode an — dort werden die beiden
// Fragmente des Einstiegs geprüft (#einstieg-kasten, #einstieg-lese) und
// zugleich, dass das alte Gitter NICHT mehr auftaucht.

// TestCockpitBody_NoTabRemnants ist entfallen — dieselbe Nachfolge: der
// Handler-Test prüft die Abwesenheit von pill-tabs und data-tabstrip auf der
// Registerseite.

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
	// Der Bearbeiten-Dialog wird von CockpitMain montiert, damit er auch
	// nach einem #cockpit-main-Tausch wieder da ist — die SEITE hat ihn nie
	// getragen.
	body := renderToBuf(t, ctx, CockpitMain(d))

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

// TestNodeCockpitShell_IncludesNodeContent ist entfallen: die Hülle
// nodeCockpitShell gibt es nicht mehr. Was sie prüfte — dass die Hülle den
// Seiteninhalt einschließt — prüft jetzt der Handler-Test am fertigen
// HTTP-Ergebnis, also näher an der Wirklichkeit.
