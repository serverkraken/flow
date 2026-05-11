package cli

import (
	"context"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/browse"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// runBrowse is the production handler. It is a package-level variable so
// tests can swap in a no-op and verify the cobra wiring without launching
// a real Bubble Tea program against a (non-existent) TTY.
var runBrowse = func(ctx context.Context, deps Deps, cwd string) error {
	var currentRepo domain.CanonicalURL
	if info, err := deps.Repo.Detect(ctx, cwd); err == nil {
		currentRepo = info.URL
	}

	writeCmd := buildWriteCmd(cwd)

	// Live-Palette laden (overlayed @tn_*-tmux-Optionen) und in alle
	// drei Kompendium-TUI-Packages durchreichen, BEVOR browse.New die
	// Styles konsumiert. theme.Init() konfiguriert den TrueColor-
	// Renderer für tmux/Ghostty etc.; theme.Load() liest @tn_* einmal.
	tk.Init()
	pal := tk.Load()
	browse.SetPalette(pal)
	markdown_overlay.SetPalette(pal)
	writepicker.SetPalette(pal)

	m := browse.New(deps.ListNotes, deps.Store, deps.DeleteNote, currentRepo, deps.EditCmd, writeCmd)
	if deps.IndexPath != "" {
		m = m.WithIndexAge(indexAgeFromFile(deps.IndexPath))
	}
	if deps.RenderBacklinks != nil {
		m = m.WithBacklinks(backlinksFromUsecase(ctx, deps.RenderBacklinks))
	}
	// Note: tea.WithMouseCellMotion was deliberately removed —
	// enabling DEC mode 1002 puts the terminal in a state where
	// drag-to-select text gets eaten by the program instead of
	// reaching the terminal/tmux selection engine. The user couldn't
	// copy text out of a note any more. Keyboard scroll (j/k,
	// ctrl+u/d) covers the only navigation the mouse-wheel handler
	// did, and tmux/terminal handle wheel scrolling natively.
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := p.Run()
	return err
}

// backlinksFromUsecase returns a BacklinksFunc closure backed by
// RenderBacklinks. Used by the browse model to populate the
// "Referenced by" footer in the full-screen viewer. Errors collapse
// to an empty slice — a missing footer is better than a torn-up
// preview when the index is mid-rebuild.
func backlinksFromUsecase(ctx context.Context, uc *usecase.RenderBacklinks) browse.BacklinksFunc {
	return func(id domain.ID) []usecase.BacklinkRef {
		out, err := uc.Execute(ctx, usecase.RenderBacklinksInput{NoteID: id})
		if err != nil {
			return nil
		}
		return out.Backlinks
	}
}

// indexAgeFromFile returns an IndexAgeFunc closure that reports the
// SQLite index's last on-disk write time via os.Stat. Missing or
// unreadable files yield a zero time, which the status bar treats as
// "unknown" and hides the indicator.
func indexAgeFromFile(path string) browse.IndexAgeFunc {
	return func() time.Time {
		st, err := os.Stat(path)
		if err != nil {
			return time.Time{}
		}
		return st.ModTime()
	}
}

// buildWriteCmd returns a closure that re-spawns the running binary
// with the matching `kompendium new <type>` subcommand for the
// picker's harvested Result. Browse runs the picker in-process now
// (the nested-bubbletea-via-tea.ExecProcess approach failed at TTY
// negotiation in bubbletea v1.3.x), so all this factory has to fork
// is the plain non-tea CLI that creates the file + opens nvim.
//
// Using os.Executable() (not os.Args[0]) keeps the nested process
// pointing at the same binary even when the user launched the
// command via a symlink or a relative path. After the K1-K5
// integration there is no standalone `kompendium` binary anymore;
// the executable is always `flow` and `new` lives under
// `flow kompendium new`. Pre-fix this factory built `<exe> new daily`
// (the standalone-kompendium-era arg layout), which under flow
// surfaces as »unknown command "new" for "flow"« and the subprocess
// exits 1 — exactly the symptom users hit on the n keypress.
func buildWriteCmd(cwd string) browse.WriteCmdFunc {
	return func(r writepicker.Result) *exec.Cmd {
		exe, err := os.Executable()
		if err != nil || exe == "" {
			exe = "flow"
		}
		var cmd *exec.Cmd
		switch r.Choice {
		case writepicker.ChoiceDaily:
			cmd = exec.Command(exe, "kompendium", "new", "daily")
		case writepicker.ChoiceProject:
			cmd = exec.Command(exe, "kompendium", "new", "project", "--cwd", cwd)
		case writepicker.ChoiceFree:
			cmd = exec.Command(exe, "kompendium", "new", "free", r.Slug)
		default:
			return nil
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd
	}
}

func newBrowseCmd(deps Deps) *cobra.Command {
	var cwd string
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Open the Bubble Tea browse view",
		Long: "Launch the interactive browser. Project notes for the cwd's repo (when in a git " +
			"repository) are promoted to the top tier. Tab cycles the type filter, / opens search, " +
			"q quits.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := cwd
			if resolved == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				resolved = wd
			}
			return runBrowse(cmd.Context(), deps, resolved)
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "working directory for repo detection (default: current)")
	return cmd
}
