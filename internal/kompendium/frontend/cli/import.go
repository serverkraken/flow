package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newImportCmd(deps Deps) *cobra.Command {
	var (
		conflict string
		bundle   bool
	)
	cmd := &cobra.Command{
		Use:   "import <archive>",
		Short: "Extract a tar.gz or merge a git bundle into the local notebook",
		Long: "Default: extract a tar.gz into the notebook with --on-conflict deciding what to do " +
			"about pre-existing files (abort | newer | manual). With --bundle, fetch + merge a git " +
			"bundle into the current branch; conflicts surface as standard git merge markers and " +
			"--on-conflict is ignored.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bundle {
				out, err := deps.ImportBundle.Execute(cmd.Context(), usecase.ImportBundleInput{BundlePath: args[0]})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Imported %s → %s (bundle)\n", args[0], out.Target)
				return err
			}
			mode, err := parseConflictMode(conflict)
			if err != nil {
				return err
			}
			out, err := deps.ImportTar.Execute(cmd.Context(), usecase.ImportTarInput{
				Archive: args[0],
				Mode:    mode,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Imported %s → %s\n", args[0], out.Target)
			return err
		},
	}
	cmd.Flags().StringVar(&conflict, "on-conflict", "abort", "tar conflict resolution: abort | newer | manual")
	cmd.Flags().BoolVar(&bundle, "bundle", false, "treat <archive> as a git bundle (fetch + merge)")
	return cmd
}

func parseConflictMode(s string) (ports.ConflictMode, error) {
	switch s {
	case "", "abort":
		return ports.ConflictAbort, nil
	case "newer":
		return ports.ConflictNewer, nil
	case "manual":
		return ports.ConflictManual, nil
	}
	return 0, fmt.Errorf("invalid --on-conflict %q (want abort, newer, or manual)", s)
}
