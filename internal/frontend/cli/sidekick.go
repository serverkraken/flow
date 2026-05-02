package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	tk "github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/sidekick"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

// SidekickDeps is the dependency bundle for the `flow sidekick` cobra
// subcommand. The four screen factories are required — each one builds
// its screen against the live theme palette read inside RunE so the
// palette is loaded exactly once per program run, after `theme.Init()`.
// Building a screen at process startup would force `flow worktime status`
// and other non-TUI verbs to make spurious `tmux show-options` calls.
type SidekickDeps struct {
	FlowState  ports.FlowStateStore
	Cheatsheet func(tk.Palette) tea.Model
	Palette    func(tk.Palette) tea.Model
	Projects   func(tk.Palette) tea.Model
	Worktime   func(tk.Palette) tea.Model
}

// NewSidekickCmd constructs the `flow sidekick` cobra subcommand. It
// loads the persisted UI state via FlowStateStore, runs the bubbletea
// program, and persists state on graceful exit.
func NewSidekickCmd(deps SidekickDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "sidekick",
		Short:        "Run as tmux sidekick panel",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			tk.Init()
			pal := tk.Load()

			fs, err := preflightSidekick(deps.FlowState)
			if err != nil {
				return err
			}

			m := sidekick.New(pal, fs, sidekick.Deps{
				Palette:    deps.Palette(pal),
				Projects:   deps.Projects(pal),
				Worktime:   deps.Worktime(pal),
				Cheatsheet: deps.Cheatsheet(pal),
			})
			prog := tea.NewProgram(m, tea.WithAltScreen())
			final, err := prog.Run()
			if err != nil {
				return err
			}
			if sm, ok := final.(sidekick.Model); ok {
				_ = deps.FlowState.Save(sm.CurrentState())
			}
			return nil
		},
	}
}

// preflightSidekick reads the persisted FlowState and applies the
// one-shot next-screen override if present. Extracted from RunE so the
// pre-bubbletea logic can be unit-tested without entering
// tea.NewProgram.Run() — that call blocks on a real TTY (and only
// short-circuits in CI where no TTY is attached).
func preflightSidekick(store ports.FlowStateStore) (domain.FlowState, error) {
	state, err := store.Load()
	if err != nil {
		return domain.FlowState{}, err
	}
	next, err := store.ConsumeNextScreen()
	if err != nil {
		return domain.FlowState{}, err
	}
	if next != "" {
		state.Screen = next
		state.Filter = ""
		state.Cursor = 0
	}
	return state, nil
}
