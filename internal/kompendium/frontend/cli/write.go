package cli

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// runPicker is the production handler that launches the Bubble Tea picker
// and returns the user's choice. Tests swap it out so the CLI wiring can be
// covered without spinning up tea against a non-existent TTY.
var runPicker = func(ctx context.Context, allowProject bool) (writepicker.Result, error) {
	m := writepicker.New(allowProject)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		return writepicker.Result{}, err
	}
	pm, ok := finalModel.(writepicker.Model)
	if !ok {
		return writepicker.Result{}, fmt.Errorf("write picker returned unexpected model %T", finalModel)
	}
	return pm.Result(), nil
}

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
