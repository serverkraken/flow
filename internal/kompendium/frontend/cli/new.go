package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// wrapProjectErr surfaces hand-written hints for the two project-creation
// failures users actually see in the wild. Any other error passes through
// untouched so cobra (and tests) see the original.
func wrapProjectErr(err error) error {
	switch {
	case errors.Is(err, ports.ErrNotInRepo):
		return fmt.Errorf("%w — cd into a repository, or use `kompendium new free <slug>` for a standalone note", err)
	case errors.Is(err, ports.ErrRepoHasNoRemote):
		return fmt.Errorf("%w — project notes need an `origin` remote for a stable cross-machine ID; use `kompendium new free <slug>` for local-only repos", err)
	}
	return err
}

func newNewCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create or open a daily, project, or free note",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newNewDailyCmd(deps),
		newNewProjectCmd(deps),
		newNewFreeCmd(deps),
	)
	return cmd
}

func newNewDailyCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "daily",
		Short: "Create or open today's daily note",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.CreateDaily.Execute(cmd.Context())
			if err != nil {
				return wrapAuthErr(err)
			}
			return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created)
		},
	}
}

func newNewProjectCmd(deps Deps) *cobra.Command {
	var cwd string
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Create or open today's project note for the current repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := cwd
			if resolved == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				resolved = wd
			}
			out, err := deps.CreateProject.Execute(cmd.Context(), usecase.CreateProjectInput{Cwd: resolved})
			if err != nil {
				return wrapAuthErr(wrapProjectErr(err))
			}
			return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created)
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "working directory to detect repo from (default: current)")
	return cmd
}

func newNewFreeCmd(deps Deps) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "free <slug>",
		Short: "Create or open a free-form note under notes/<slug>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := deps.CreateFree.Execute(cmd.Context(), usecase.CreateFreeInput{
				Slug:  args[0],
				Title: title,
			})
			if err != nil {
				return wrapAuthErr(err)
			}
			return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "frontmatter title for newly created notes")
	return cmd
}
