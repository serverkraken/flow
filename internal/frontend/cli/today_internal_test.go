package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Internal test (package cli, not cli_test) so it can swap the
// package-level runWorktimeToday hook without exposing it through the
// public API. Mirrors the kompendium-cli browse_test pattern.

func swapRunWorktimeToday(replacement func(context.Context, WorktimeDeps) error) func() {
	prev := runWorktimeToday
	runWorktimeToday = replacement
	return func() { runWorktimeToday = prev }
}

func TestToday_NilScreenReturnsError(t *testing.T) {
	cmd := NewWorktimeCmd(WorktimeDeps{ /* Screen left nil */ })
	cmd.SetArgs([]string{"today"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when Screen factory is nil")
	}
	if !strings.Contains(err.Error(), "screen factory") {
		t.Errorf("error message should hint at the missing factory; got %q", err)
	}
}

// TestToday_RunsScreenFactory verifies the cobra wiring hands the
// pal-keyed Screen factory through to the runner without spinning up
// a real Bubble Tea program. Stubs runWorktimeToday so the production
// path stays untouched while the test exercises every line of the
// RunE that isn't behind tea.NewProgram.
func TestToday_RunsScreenFactory(t *testing.T) {
	t.Cleanup(swapRunWorktimeToday(func(_ context.Context, d WorktimeDeps) error {
		if d.Screen == nil {
			t.Errorf("runWorktimeToday received nil Screen factory")
		}
		// Resolving the factory once is enough to prove the wiring; we
		// don't actually run the model.
		_ = d.Screen(tk.Palette{})
		return nil
	}))

	called := 0
	deps := WorktimeDeps{
		Screen: func(_ tk.Palette) tea.Model {
			called++
			return nil
		},
	}
	cmd := NewWorktimeCmd(deps)
	cmd.SetArgs([]string{"today"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("today: %v", err)
	}
	if called != 1 {
		t.Errorf("Screen factory called %d times, want 1", called)
	}
}

// Note: the production runWorktimeToday body (tk.Init + tk.Load +
// tea.NewProgram + prog.Run) is intentionally not unit-tested. tmux's
// run-shell environment provides /dev/tty + a real terminal; bats
// tests stub `flow worktime today` at the binary boundary; an
// in-process test would need a synthetic TTY which Go test runners
// don't supply. The cobra wiring + nil-screen guard + error
// propagation are covered above; the TUI launch itself is covered by
// runtime smoke-test (`flow worktime today` in a tmux pane).

// TestToday_RunnerErrorBubbles: an error from runWorktimeToday must
// surface up the cobra return path so the dotfiles' sidekick view can
// distinguish "user pressed q" (success) from "tea.Program crashed"
// (non-zero exit triggers the view's restart-loop intent).
func TestToday_RunnerErrorBubbles(t *testing.T) {
	want := errors.New("forced runner error")
	t.Cleanup(swapRunWorktimeToday(func(_ context.Context, _ WorktimeDeps) error { return want }))

	deps := WorktimeDeps{Screen: func(_ tk.Palette) tea.Model { return nil }}
	cmd := NewWorktimeCmd(deps)
	cmd.SetArgs([]string{"today"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if !errors.Is(err, want) {
		t.Errorf("got %v, want wrapped %v", err, want)
	}
}
