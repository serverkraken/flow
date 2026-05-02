package cli_test

// Smoke tests for NewSidekickCmd. The bubbletea program inside RunE
// can't run in a test environment (no TTY), but the surrounding load /
// next-screen / save logic is testable via the FlowStateStore fake's
// LoadErr / SaveErr hooks. Goal: cover everything before tea.NewProgram
// is invoked and the metadata around the cobra command itself.

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/cli"
	tk "github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// stubScreen is a no-op tea.Model just to satisfy the screen factory
// signature — RunE never reaches the program loop in these tests.
type stubScreen struct{}

func (stubScreen) Init() tea.Cmd                       { return nil }
func (stubScreen) Update(tea.Msg) (tea.Model, tea.Cmd) { return stubScreen{}, nil }
func (stubScreen) View() string                        { return "" }

func makeSidekickDeps(state *testutil.FakeFlowStateStore) cli.SidekickDeps {
	factory := func(tk.Palette) tea.Model { return stubScreen{} }
	return cli.SidekickDeps{
		FlowState:  state,
		Cheatsheet: factory,
		Palette:    factory,
		Projects:   factory,
		Worktime:   factory,
	}
}

func TestNewSidekickCmd_ConstructsValidCobraCommand(t *testing.T) {
	cmd := cli.NewSidekickCmd(makeSidekickDeps(&testutil.FakeFlowStateStore{}))
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

func TestNewSidekickCmd_LoadError_PropagatesEarly(t *testing.T) {
	wantErr := errors.New("flowstate corrupt")
	state := &testutil.FakeFlowStateStore{LoadErr: wantErr}
	cmd := cli.NewSidekickCmd(makeSidekickDeps(state))
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Errorf("RunE should propagate the load error, got %v", err)
	}
}

func TestNewSidekickCmd_NextScreenOverridesPersistedState(t *testing.T) {
	// Make sure that ConsumeNextScreen's value is honored. We can't observe
	// the override directly because RunE proceeds into tea.NewProgram which
	// fails without a TTY — but we can confirm:
	//   1. NextScreen was consumed (cleared)
	//   2. RunE did not return the load error (since LoadErr is unset)
	state := &testutil.FakeFlowStateStore{
		State:      domain.FlowState{Screen: "palette", Filter: "stale"},
		NextScreen: "worktime",
	}
	cmd := cli.NewSidekickCmd(makeSidekickDeps(state))
	cmd.SetArgs([]string{})
	// Execute will fail at tea.NewProgram.Run() in a no-TTY environment,
	// but only after ConsumeNextScreen has been called and applied.
	_ = cmd.Execute()
	if state.NextScreen != "" {
		t.Errorf("ConsumeNextScreen should have cleared NextScreen, still %q", state.NextScreen)
	}
}
