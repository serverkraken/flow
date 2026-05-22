package cli

// Smoke + behaviour tests for the sidekick subcommand. Cobra's RunE
// proceeds into tea.NewProgram.Run() which blocks on a real TTY, so the
// testable preflight (FlowState load + next-screen apply) lives in
// preflightSidekick and the tests target that helper directly.
// White-box per the documented exception for unexported helpers.

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// stubScreen is a no-op tea.Model just to satisfy the screen factory
// signature in metadata-only tests.
type stubScreen struct{}

func (stubScreen) Init() tea.Cmd                       { return nil }
func (stubScreen) Update(tea.Msg) (tea.Model, tea.Cmd) { return stubScreen{}, nil }
func (stubScreen) View() tea.View                      { return tea.NewView("") }

func makeSidekickDeps(state *testutil.FakeFlowStateStore) SidekickDeps {
	factory := func(tk.Palette) tea.Model { return stubScreen{} }
	return SidekickDeps{
		FlowState:  state,
		Cheatsheet: factory,
		Palette:    factory,
		Projects:   factory,
		Worktime:   factory,
	}
}

func TestNewSidekickCmd_ConstructsValidCobraCommand(t *testing.T) {
	cmd := NewSidekickCmd(makeSidekickDeps(&testutil.FakeFlowStateStore{}))
	if cmd == nil {
		t.Fatal("expected a non-nil command")
	}
	if cmd.Use != "sidekick" {
		t.Errorf("Use: got %q want sidekick", cmd.Use)
	}
	if cmd.Short == "" {
		t.Errorf("Short should describe the command")
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage should be true so cobra doesn't print the usage block on RunE error")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE should be set")
	}
}

func TestPreflightSidekick_LoadError_PropagatesEarly(t *testing.T) {
	wantErr := errors.New("flowstate corrupt")
	state := &testutil.FakeFlowStateStore{LoadErr: wantErr}
	if _, err := preflightSidekick(state); !errors.Is(err, wantErr) {
		t.Errorf("preflightSidekick should propagate the load error, got %v", err)
	}
}

func TestPreflightSidekick_NextScreenOverridesPersistedState(t *testing.T) {
	state := &testutil.FakeFlowStateStore{
		State:      domain.FlowState{Screen: "palette", Filter: "stale", Cursor: 7},
		NextScreen: "worktime",
	}
	fs, err := preflightSidekick(state)
	if err != nil {
		t.Fatalf("preflightSidekick: %v", err)
	}
	if fs.Screen != "worktime" {
		t.Errorf("Screen: got %q want worktime", fs.Screen)
	}
	if fs.Filter != "" {
		t.Errorf("Filter: got %q want empty (cleared by next-screen override)", fs.Filter)
	}
	if fs.Cursor != 0 {
		t.Errorf("Cursor: got %d want 0 (cleared by next-screen override)", fs.Cursor)
	}
	if state.NextScreen != "" {
		t.Errorf("ConsumeNextScreen should have cleared NextScreen, still %q", state.NextScreen)
	}
}
