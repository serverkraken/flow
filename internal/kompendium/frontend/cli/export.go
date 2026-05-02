package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newExportCmd(deps Deps) *cobra.Command {
	var bundle bool
	cmd := &cobra.Command{
		Use:   "export <out>",
		Short: "Export the notebook as a tar.gz snapshot or git bundle",
		Long: "Default: tar.gz snapshot of the notebook (excluding .git/), suitable for one-shot " +
			"transfer to a fresh machine. With --bundle, write a git bundle so the recipient can " +
			"import incrementally and resolve conflicts via standard git merge.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bundle {
				out, err := deps.ExportBundle.Execute(cmd.Context(), usecase.ExportBundleInput{OutPath: args[0]})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported %s → %s (bundle)\n", out.Source, out.OutPath)
				return err
			}
			out, err := deps.ExportTar.Execute(cmd.Context(), usecase.ExportTarInput{OutPath: args[0]})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported %s → %s\n", out.Source, out.OutPath)
			return err
		},
	}
	cmd.Flags().BoolVar(&bundle, "bundle", false, "use git-bundle for incremental + mergeable transfer")
	return cmd
}
