package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestTimerWidget_IdleRendersStartFormAndQuickCreate verifies the idle desktop
// card: a start form posting to /ui/timer/start, the bookable-node select, and
// the newProject quick-create field.
func TestTimerWidget_IdleRendersStartFormAndQuickCreate(t *testing.T) {
	ctx := context.Background()
	vm := TimerWidgetVM{Bookable: []domain.Node{{ID: "n1", Name: "flow"}}}
	body := renderToBuf(t, ctx, TimerWidget(vm))

	for _, want := range []string{`hx-post="/ui/timer/start"`, `name="newProject"`, `value="n1"`, "flow"} {
		if !strings.Contains(body, want) {
			t.Errorf("idle widget missing %q, got: %s", want, body)
		}
	}
	if strings.Contains(body, "data-timer") {
		t.Errorf("idle widget must not render the live clock, got: %s", body)
	}
}

// TestTimerWidget_RunningRendersClockAndNodePill verifies the running (bound)
// state: the live clock element seeded with BaseSeconds, a linked node pill,
// and a stop form — the switch details stay collapsed but present.
func TestTimerWidget_RunningRendersClockAndNodePill(t *testing.T) {
	ctx := context.Background()
	vm := TimerWidgetVM{
		Running: true, SessionID: "s1", NodeID: "n1", NodeName: "flow",
		NodeKind: domain.KindRepo, BaseSeconds: 90,
	}
	body := renderToBuf(t, ctx, TimerWidget(vm))

	for _, want := range []string{"data-timer", `data-base="90"`, `href="/nodes/n1"`, "flow", `hx-post="/ui/timer/stop"`, `hx-post="/ui/timer/switch"`} {
		if !strings.Contains(body, want) {
			t.Errorf("running widget missing %q, got: %s", want, body)
		}
	}
	// 90s -> fmtClockHMS renders "00:01:30", the initial clock text before JS
	// binds. Must match the live-timer script's data-timer-fmt="clock" branch
	// (components/base.templ: p(h)+':'+p(m)+':'+p(s), zero-padded hours too)
	// or the JS's first tick visibly reformats the text (SSR/JS parity bug).
	if !strings.Contains(body, "00:01:30") {
		t.Errorf("running widget missing initial clock text, got: %s", body)
	}
	if strings.Contains(body, "0h 01m 30s") {
		t.Errorf("running widget must not use the non-clock fmtSecsClock shape for the data-timer-fmt=clock span, got: %s", body)
	}
}

// TestTimerWidget_UnboundRendersNeedNodeSelect verifies the running-unbound
// state: the stop form embeds the mandatory node select (label = timer.needNode)
// instead of letting Stop silently no-op.
func TestTimerWidget_UnboundRendersNeedNodeSelect(t *testing.T) {
	ctx := context.Background()
	vm := TimerWidgetVM{
		Running: true, Unbound: true, SessionID: "s1", BaseSeconds: 30,
		Bookable: []domain.Node{{ID: "n1", Name: "flow"}},
	}
	body := renderToBuf(t, ctx, TimerWidget(vm))

	if !strings.Contains(body, "Zum Stoppen Projekt wählen") {
		t.Errorf("unbound widget missing timer.needNode select label, got: %s", body)
	}
	if !strings.Contains(body, "ohne Projekt") {
		t.Errorf("unbound widget missing timer.unbound marker, got: %s", body)
	}
	if strings.Contains(body, `href="/nodes/`) {
		t.Errorf("unbound widget must not link a node, got: %s", body)
	}
}

// TestTimerWidget_ErrRendersInlineNeverPopup pins the no-popup rule: an error
// renders as an inline alert banner, never as window.alert/confirm/prompt.
func TestTimerWidget_ErrRendersInlineNeverPopup(t *testing.T) {
	ctx := context.Background()
	vm := TimerWidgetVM{Err: "Zum Stoppen Projekt wählen", Bookable: []domain.Node{{ID: "n1", Name: "flow"}}}
	body := renderToBuf(t, ctx, TimerWidget(vm))

	if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "Zum Stoppen Projekt wählen") {
		t.Errorf("error must render inline with role=alert, got: %s", body)
	}
	if strings.Contains(body, "window.alert") || strings.Contains(body, "window.confirm") || strings.Contains(body, "window.prompt") {
		t.Errorf("error must never surface as a browser popup, got: %s", body)
	}
}

// TestTimerChip_RunningRendersMiniTimerAndDialogTrigger verifies the mobile
// chip: a compact live clock plus the dialog-open trigger that reveals the
// full widget (no browser popups — Kristall dialogs only), and that the
// trigger button carries an accessible name (its visible content is only a
// clock digit string, otherwise announced with no label).
func TestTimerChip_RunningRendersMiniTimerAndDialogTrigger(t *testing.T) {
	ctx := context.Background()
	vm := TimerWidgetVM{Running: true, SessionID: "s1", BaseSeconds: 45}
	body := renderToBuf(t, ctx, TimerChip(vm))

	for _, want := range []string{"data-mini-timer", `data-dialog-open="timer-sheet"`, `id="timer-sheet"`, `aria-label="Timer"`} {
		if !strings.Contains(body, want) {
			t.Errorf("running chip missing %q, got: %s", want, body)
		}
	}
	// 45s -> fmtClockHMS renders "00:00:45", matching the [data-mini-timer]
	// JS branch (p(h)+':'+p(m)+':'+p(s), zero-padded hours too) exactly.
	if !strings.Contains(body, "00:00:45") {
		t.Errorf("running chip missing initial mini-timer clock text, got: %s", body)
	}
}

// TestTimerChip_IdleRendersPlaceholder verifies the idle chip shows the
// "start a timer" affordance rather than a clock, and still carries the
// accessible name on its (single, state-independent) trigger button.
func TestTimerChip_IdleRendersPlaceholder(t *testing.T) {
	ctx := context.Background()
	body := renderToBuf(t, ctx, TimerChip(TimerWidgetVM{}))

	if strings.Contains(body, "data-mini-timer") {
		t.Errorf("idle chip must not render the live clock, got: %s", body)
	}
	if !strings.Contains(body, "Timer") {
		t.Errorf("idle chip missing timer.title label, got: %s", body)
	}
	if !strings.Contains(body, `aria-label="Timer"`) {
		t.Errorf("idle chip trigger missing accessible name, got: %s", body)
	}
}
