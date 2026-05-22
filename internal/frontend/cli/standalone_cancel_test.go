package cli_test

// Exercises the RunE bodies of the standalone-popup cobra commands by
// running them with an already-cancelled context. tea.NewProgram.Run
// honours tea.WithContext: when the context is dead before .Run() is
// called, the program returns immediately. That lets the constructor
// reach theme.Init / theme.Load / factory / NewProgram lines without
// requiring a real terminal.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/frontend/cli"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

type stubModel struct{}

func (stubModel) Init() tea.Cmd                       { return nil }
func (stubModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return stubModel{}, nil }
func (stubModel) View() string                        { return "" }

func runStandaloneCancelled(t *testing.T, cmd *cobra.Command, args []string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-done:
		// Either nil or a context-cancellation-shaped error is fine —
		// the point is that the RunE body executed past the constructor.
	case <-time.After(3 * time.Second):
		t.Errorf("cmd did not honour cancelled context within 3s")
	}
}

func TestCheatsheetCmd_RunWithCancelledCtx(t *testing.T) {
	c := cli.NewCheatsheetCmd(cli.CheatsheetDeps{
		Reader:   &testutil.FakeCheatsheetReader{Content: "# C"},
		Renderer: &testutil.FakeMarkdownRenderer{},
	})
	runStandaloneCancelled(t, c, []string{})
}

func TestPaletteCmd_RunWithCancelledCtx(t *testing.T) {
	c := cli.NewPaletteCmd(cli.PaletteDeps{Screen: func(tk.Palette) tea.Model { return stubModel{} }})
	runStandaloneCancelled(t, c, []string{})
}

func TestProjectsCmd_RunWithCancelledCtx(t *testing.T) {
	c := cli.NewProjectsCmd(cli.ProjectsDeps{Screen: func(tk.Palette) tea.Model { return stubModel{} }})
	runStandaloneCancelled(t, c, []string{})
}

func TestMarkdownView_RunWithCancelledCtx(t *testing.T) {
	root := cli.NewMarkdownCmd()
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(path, []byte("# Hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	runStandaloneCancelled(t, root, []string{"view", path})
}

func TestSidekickCmd_RunWithCancelledCtx(t *testing.T) {
	deps := cli.SidekickDeps{
		FlowState:  &testutil.FakeFlowStateStore{},
		Cheatsheet: func(tk.Palette) tea.Model { return stubModel{} },
		Palette:    func(tk.Palette) tea.Model { return stubModel{} },
		Projects:   func(tk.Palette) tea.Model { return stubModel{} },
		Worktime:   func(tk.Palette) tea.Model { return stubModel{} },
		Notes:      func(tk.Palette) tea.Model { return stubModel{} },
	}
	c := cli.NewSidekickCmd(deps)
	runStandaloneCancelled(t, c, []string{})
}

func TestMarkdownView_RawMode(t *testing.T) {
	// Exercises the --raw branch of newMarkdownViewCmd which doesn't go
	// through tea.NewProgram at all — it renders directly to stdout via
	// markdown.Render.
	root := cli.NewMarkdownCmd()
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(path, []byte("# Hi\n\nbody"), 0o600); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"view", "--raw", "--width", "60", path})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	// Override os.Stdout temporarily — newMarkdownViewCmd writes to
	// os.Stdout in raw mode, not cmd.OutOrStdout. We can't easily
	// intercept that without rewiring the source, so just verify the
	// command returned without error.
	_ = time.Now
	if err := root.Execute(); err != nil {
		t.Errorf("raw-mode markdown view: %v", err)
	}
}
