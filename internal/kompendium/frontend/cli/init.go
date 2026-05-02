package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInitCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise the notebook as a git repository",
		Long: "Make $NOTES_DIR (or ~/notes) a git working tree with an initial commit. " +
			"Idempotent: already-initialised notebooks report as such without touching the repo.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.InitNotebook.Execute(cmd.Context())
			if err != nil {
				return err
			}
			verb := "Initialised"
			if out.AlreadyInitialized {
				verb = "Already a git repo:"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", verb, out.Root)
			return err
		},
	}
}
