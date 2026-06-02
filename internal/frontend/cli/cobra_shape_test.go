package cli

// Shape tests for the standalone cobra constructors whose RunE bodies
// proceed into tea.NewProgram and therefore can't be exercised inside
// `go test`. The constructors themselves still own measurable code
// (cobra wiring, flag defaults, Args validators) — these tests prove
// the wiring matches what the tmux-popup callers expect.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// — Cheatsheet —

func TestNewCheatsheetCmd_Shape(t *testing.T) {
	deps := CheatsheetDeps{
		Reader:   &testutil.FakeCheatsheetReader{Content: "# Cheatsheet"},
		Renderer: &testutil.FakeMarkdownRenderer{},
	}
	cmd := NewCheatsheetCmd(deps)
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "cheatsheet" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
	if cmd.Short == "" {
		t.Errorf("Short must describe the command")
	}
}

// — Palette —

func TestNewPaletteCmd_Shape(t *testing.T) {
	factory := func(tk.Palette) tea.Model { return stubScreen{} }
	cmd := NewPaletteCmd(PaletteDeps{Screen: factory})
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "palette" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
}

// — Projects —

func TestNewProjectsCmd_Shape(t *testing.T) {
	factory := func(tk.Palette) tea.Model { return stubScreen{} }
	cmd := NewProjectsCmd(ProjectsDeps{Screen: factory})
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "projects" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
}

// — markdownViewerProgram —

func TestMarkdownViewerProgram_DelegatesToOverlay(t *testing.T) {
	// Build an overlay around a no-op renderer. Init/Update/View should
	// proxy unchanged to the embedded markdown_overlay.Model.
	render := func(src string, _ int) string { return src }
	overlay := markdown_overlay.New(render,
		markdown_overlay.WithTitle("t"),
		markdown_overlay.WithSource("# hello"),
	)
	prog := markdownViewerProgram{inner: overlay}
	// Init returns the overlay's tea.Cmd (may be nil for the no-search variant).
	_ = prog.Init()
	// View renders without panic. Before WindowSizeMsg the overlay may
	// produce an empty string — that's still a valid pass-through.
	_ = prog.View()
	// Update with a WindowSizeMsg sizes the overlay; View should now
	// produce some output (or remain empty if the overlay defers paints).
	updated, _ := prog.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = updated.View()
	// ExitMsg → tea.Quit branch.
	next, cmd := prog.Update(markdown_overlay.ExitMsg{})
	if next == nil || cmd == nil {
		t.Errorf("ExitMsg must produce a quit cmd, got next=%v cmd=%v", next, cmd)
	}
}

// — markdown.Render via NewMarkdownCmd raw mode flag wiring —

func TestNewMarkdownCmd_RawFlagsDefined(t *testing.T) {
	root := NewMarkdownCmd()
	sub, _, err := root.Find([]string{"view"})
	if err != nil || sub == nil {
		t.Fatalf("`view` not found: %v", err)
	}
	if f := sub.Flags().Lookup("raw"); f == nil {
		t.Errorf("--raw flag missing")
	}
	if f := sub.Flags().Lookup("width"); f == nil {
		t.Errorf("--width flag missing")
	}
	// Compile-time sanity: package is reachable in case the helper imports change.
	_, _ = markdown.Render("", 0)
	// Also verify --raw default value is false.
	if f := sub.Flags().Lookup("raw"); f != nil && f.DefValue != "false" {
		t.Errorf("--raw default: %q, want false", f.DefValue)
	}
	if f := sub.Flags().Lookup("width"); f != nil && !strings.Contains(f.DefValue, "100") {
		t.Errorf("--width default: %q, want 100", f.DefValue)
	}
}

// — Sync —

func TestNewSyncCmd_Shape(t *testing.T) {
	cmd := NewSyncCmd(SyncDeps{Controller: &stubSyncController{}})
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "sync" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}

	subNames := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subNames[sub.Use] = true
		if !sub.SilenceUsage {
			t.Errorf("subcommand %q SilenceUsage must be true", sub.Use)
		}
		if sub.RunE == nil {
			t.Errorf("subcommand %q RunE must be set", sub.Use)
		}
	}
	if !subNames["status"] {
		t.Errorf("missing subcommand 'status'")
	}
	if !subNames["force-pull"] {
		t.Errorf("missing subcommand 'force-pull'")
	}
}

// stubSyncController is a minimal fake for shape tests.
type stubSyncController struct{}

func (s *stubSyncController) Status() (ports.SyncStatus, error) {
	return ports.SyncStatus{Watermarks: map[string]int64{}}, nil
}
func (s *stubSyncController) ForcePull() error                     { return nil }
func (s *stubSyncController) AcceptServerVersion(_ int64) error    { return nil }
func (s *stubSyncController) OverwriteServerVersion(_ int64) error { return nil }
