package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newSnapshotCmd(deps Deps) *cobra.Command {
	var msg string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Stage and commit pending notebook changes",
		Long: "Run `git add . && git commit` inside the notebook with a kompendium identity. " +
			"Skips the commit when the working tree is already clean.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.SnapshotNotebook.Execute(cmd.Context(), usecase.SnapshotNotebookInput{Message: msg})
			if err != nil {
				return err
			}
			if !out.HadChanges {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to snapshot — working tree clean.")
			} else {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Snapshot committed.")
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "", "commit message (default: \"kompendium snapshot\")")
	return cmd
}
