package cli

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// runPicker is the production handler that launches the Bubble Tea picker
// and returns the user's choice. Tests swap it out so the CLI wiring can be
// covered without spinning up tea against a non-existent TTY.
//
// The picker now emits writepicker.DoneMsg instead of tea.Quit (so it can
// be embedded in a hosting model like kompendium browse without forking
// a subprocess). For this standalone path we wrap the picker in a tiny
// tea.Model adapter that converts DoneMsg → tea.Quit, mirroring the
// existing view.ExitMsg pattern.
var runPicker = func(ctx context.Context, allowProject bool) (writepicker.Result, error) {
	// Live-Palette in das writepicker-Package durchreichen, damit der
	// Standalone-Picker den User-tmux-Overlay (@tn_*) erbt.
	tk.Init()
	writepicker.SetPalette(tk.Load())
	host := pickerHost{inner: writepicker.New(allowProject)}
	p := tea.NewProgram(host, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		return writepicker.Result{}, err
	}
	final, ok := finalModel.(pickerHost)
	if !ok {
		return writepicker.Result{}, fmt.Errorf("write picker returned unexpected model %T", finalModel)
	}
	return final.result, nil
}

// pickerHost adapts the writepicker into a standalone tea.Program. The
// host catches DoneMsg, stashes the Result, and returns tea.Quit. The
// inner picker's typed Update is forwarded otherwise.
type pickerHost struct {
	inner  writepicker.Model
	result writepicker.Result
}

func (h pickerHost) Init() tea.Cmd { return h.inner.Init() }

func (h pickerHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if d, ok := msg.(writepicker.DoneMsg); ok {
		h.result = d.Result
		return h, tea.Quit
	}
	next, cmd := h.inner.Update(msg)
	if pm, ok := next.(writepicker.Model); ok {
		h.inner = pm
	}
	return h, cmd
}

func (h pickerHost) View() string { return h.inner.View() }

func newWriteCmd(deps Deps) *cobra.Command {
	var cwd string
	cmd := &cobra.Command{
		Use:   "write",
		Short: "Open the Bubble Tea write picker (Daily / Project / Free)",
		Long: "Pop a small picker that lets the user choose between Daily, Project (when in a " +
			"git repository), and Free notes. Selection runs the corresponding new <type> use " +
			"case and opens the resulting note in the editor.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := resolveCwd(cwd)
			if err != nil {
				return err
			}
			allowProject := false
			if _, err := deps.Repo.Detect(cmd.Context(), resolved); err == nil {
				allowProject = true
			}

			result, err := runPicker(cmd.Context(), allowProject)
			if err != nil {
				return err
			}
			return dispatchPicked(cmd, deps, resolved, result)
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "working directory for repo detection (default: current)")
	return cmd
}

func resolveCwd(cwd string) (string, error) {
	if cwd != "" {
		return cwd, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cwd: %w", err)
	}
	return wd, nil
}

func dispatchPicked(cmd *cobra.Command, deps Deps, cwd string, result writepicker.Result) error {
	switch result.Choice {
	case writepicker.ChoiceCancel:
		return nil
	case writepicker.ChoiceDaily:
		out, err := deps.CreateDaily.Execute(cmd.Context())
		if err != nil {
			return err
		}
		return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created, out.Path)
	case writepicker.ChoiceProject:
		out, err := deps.CreateProject.Execute(cmd.Context(), usecase.CreateProjectInput{Cwd: cwd})
		if err != nil {
			return wrapProjectErr(err)
		}
		return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created, out.Path)
	case writepicker.ChoiceFree:
		out, err := deps.CreateFree.Execute(cmd.Context(), usecase.CreateFreeInput{Slug: result.Slug})
		if err != nil {
			return err
		}
		return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created, out.Path)
	}
	return fmt.Errorf("unknown picker choice %d", result.Choice)
}
