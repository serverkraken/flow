package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newSyncCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull and push the notebook against `origin` (the cross-machine round-trip)",
		Long: "Runs `git pull --rebase --autostash origin <branch>` followed by `git push origin " +
			"<branch>`. Uncommitted changes are auto-stashed around the pull and stay local — " +
			"run `kompendium snapshot` first to make them travel. Configure the remote once via " +
			"`kompendium remote set <url>`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.SyncNotebook.Execute(cmd.Context())
			if err != nil {
				if errors.Is(err, ports.ErrNoRemoteConfigured) {
					return fmt.Errorf("%w — run `kompendium remote set <url>` first", err)
				}
				return err
			}
			verb := "Up to date."
			switch {
			case out.Stats.Pulled && out.Stats.Pushed:
				verb = "Synced (pulled + pushed)."
			case out.Stats.Pulled:
				verb = "Pulled (push failed — see error)."
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), verb)
			return err
		},
	}
}

func newRemoteCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Show or set the notebook's `origin` remote URL",
		Long: "Without arguments, prints the configured origin URL (or `(none)` when unset). " +
			"`remote set <url>` writes a new origin so `kompendium sync` has somewhere to push to.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.ManageRemote.Get(cmd.Context())
			if err != nil {
				return err
			}
			if out.URL == "" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "(none) — set with `kompendium remote set <url>`")
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), out.URL)
			return err
		},
	}
	cmd.AddCommand(newRemoteSetCmd(deps))
	return cmd
}

func newRemoteSetCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "set <url>",
		Short: "Set the notebook's `origin` remote URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := deps.ManageRemote.Set(cmd.Context(), usecase.SetInput{URL: args[0]})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Set origin to %s\n", out.URL)
			return err
		},
	}
}
