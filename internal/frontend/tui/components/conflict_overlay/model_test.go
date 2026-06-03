package conflict_overlay_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/conflict_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// resolveMsg is an arbitrary tea.Msg used to verify onResolve callbacks.
type resolveMsg struct {
	accepted bool
}

// takeoverMsg and parallelMsg are used for VariantActiveRace callbacks.
type (
	takeoverMsg struct{}
	parallelMsg struct{}
)

// dispatchKey sends a single key string to the model and returns the
// updated model plus the tea.Msg produced by any returned cmd.
func dispatchKey(m conflict_overlay.Model, key string) (conflict_overlay.Model, tea.Msg) {
	var kp tea.KeyPressMsg
	switch key {
	case "esc":
		kp = tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		kp = tea.KeyPressMsg{Text: key}
	}
	updated, cmd := m.Update(kp)
	var msg tea.Msg
	if cmd != nil {
		msg = cmd()
	}
	return updated, msg
}

// newEditModel is a test helper for VariantSessionEdit with fixed sessions.
func newEditModel() conflict_overlay.Model {
	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{
		Start:   t0,
		Stop:    t0.Add(90 * time.Minute),
		Elapsed: 90 * time.Minute,
		Tag:     "deep",
		Note:    "morning focus",
	}
	server := domain.Session{
		Start:   t0,
		Stop:    t0.Add(105 * time.Minute),
		Elapsed: 105 * time.Minute,
		Tag:     "deep",
		Note:    "morning focus (touched on phone)",
	}
	m := conflict_overlay.NewSessionEditConflict(
		local, server,
		theme.Default,
		func(accept bool) tea.Msg { return resolveMsg{accepted: accept} },
	)
	return m.SetSize(80, 24)
}

// newRaceModel is a test helper for VariantActiveRace with a fixed server session.
func newRaceModel() conflict_overlay.Model {
	srv := domain.ActiveSession{
		UserID:          "u1",
		ProjectID:       "flow",
		StartedAt:       time.Now().Add(-7 * time.Minute),
		StartedOnDevice: "notebook-b",
	}
	m := conflict_overlay.NewActiveRaceConflict(
		srv,
		theme.Default,
		func() tea.Msg { return takeoverMsg{} },
		func() tea.Msg { return parallelMsg{} },
	)
	return m.SetSize(80, 24)
}

// ── VariantSessionEdit ────────────────────────────────────────────────────

// TestUnit_SessionEdit_ThreeChoices verifies that NewSessionEditConflict
// produces a model that has exactly three choices (s, l, esc).
func TestUnit_SessionEdit_ThreeChoices(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	// The model has two user choices + implicit esc = 3 resolvable keys.
	// We probe by dispatching each and checking we get a message.
	_, msg := dispatchKey(m, "s")
	if msg == nil {
		t.Fatal("expected msg for key 's', got nil")
	}
	_, msg = dispatchKey(m, "l")
	if msg == nil {
		t.Fatal("expected msg for key 'l', got nil")
	}
	_, msg = dispatchKey(m, "esc")
	if msg == nil {
		t.Fatal("expected msg for key 'esc', got nil")
	}
}

// TestUnit_SessionEdit_SKeyInvokesResolveTrue verifies that pressing [s]
// calls onResolve(true) — server-version accepted.
func TestUnit_SessionEdit_SKeyInvokesResolveTrue(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, msg := dispatchKey(m, "s")
	rm, ok := msg.(resolveMsg)
	if !ok {
		t.Fatalf("expected resolveMsg, got %T", msg)
	}
	if !rm.accepted {
		t.Error("expected accept=true for key 's'")
	}
}

// TestUnit_SessionEdit_LKeyInvokesResolveFalse verifies that pressing [l]
// calls onResolve(false) — local version overrides server.
func TestUnit_SessionEdit_LKeyInvokesResolveFalse(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, msg := dispatchKey(m, "l")
	rm, ok := msg.(resolveMsg)
	if !ok {
		t.Fatalf("expected resolveMsg, got %T", msg)
	}
	if rm.accepted {
		t.Error("expected accept=false for key 'l'")
	}
}

// TestUnit_SessionEdit_EscReturnsCancelMsg verifies that pressing Esc
// emits a CancelMsg.
func TestUnit_SessionEdit_EscReturnsCancelMsg(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, msg := dispatchKey(m, "esc")
	if _, ok := msg.(conflict_overlay.CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", msg)
	}
}

// TestUnit_SessionEdit_UnknownKeyNoOp verifies that an unmapped key
// returns the model unchanged and no cmd.
func TestUnit_SessionEdit_UnknownKeyNoOp(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, msg := dispatchKey(m, "x")
	if msg != nil {
		t.Errorf("expected nil msg for unknown key 'x', got %T: %v", msg, msg)
	}
}

// ── VariantActiveRace ─────────────────────────────────────────────────────

// TestUnit_ActiveRace_TKeyInvokesOnTakeover verifies that [t] calls
// onTakeover.
func TestUnit_ActiveRace_TKeyInvokesOnTakeover(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	_, msg := dispatchKey(m, "t")
	if _, ok := msg.(takeoverMsg); !ok {
		t.Fatalf("expected takeoverMsg, got %T", msg)
	}
}

// TestUnit_ActiveRace_NKeyInvokesOnParallel verifies that [n] calls
// onParallel.
func TestUnit_ActiveRace_NKeyInvokesOnParallel(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	_, msg := dispatchKey(m, "n")
	if _, ok := msg.(parallelMsg); !ok {
		t.Fatalf("expected parallelMsg, got %T", msg)
	}
}

// TestUnit_ActiveRace_EscReturnsCancelMsg verifies that Esc emits CancelMsg.
func TestUnit_ActiveRace_EscReturnsCancelMsg(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	_, msg := dispatchKey(m, "esc")
	if _, ok := msg.(conflict_overlay.CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", msg)
	}
}

// TestUnit_ActiveRace_UnknownKeyNoOp verifies that an unmapped key is ignored.
func TestUnit_ActiveRace_UnknownKeyNoOp(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	_, msg := dispatchKey(m, "z")
	if msg != nil {
		t.Errorf("expected nil msg for unknown key 'z', got %T: %v", msg, msg)
	}
}

// ── WindowSizeMsg routing ─────────────────────────────────────────────────

// TestUnit_WindowSizeMsg_UpdatesDimensions verifies that a WindowSizeMsg
// updates the model dimensions.
func TestUnit_WindowSizeMsg_UpdatesDimensions(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	// Verify via View — at 100×40 we expect a non-empty view.
	if updated.View() == "" {
		t.Error("expected non-empty View() after WindowSizeMsg 100×40")
	}
}
