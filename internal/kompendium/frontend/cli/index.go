package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newIndexCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Inspect and maintain the FTS5 search index",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newIndexRebuildCmd(deps))
	return cmd
}

func newIndexRebuildCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild",
		Short: "Drop and rebuild the search index from the current notebook",
		Long: "Walks every note in the notebook, reads its body, and replaces the FTS5 index so " +
			"`kompendium search` reflects the on-disk truth. Run after `import-legacy` or any " +
			"out-of-band edits.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.RebuildIndex.Execute(cmd.Context())
			if err != nil {
				return err
			}
			// Success summary on stdout; per-note warnings on stderr so a
			// `| tee log.txt` pipe doesn't mix them and `> /dev/null`
			// doesn't silently swallow failures.
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d notes\n", out.Indexed); err != nil {
				return err
			}
			for _, e := range out.Errors {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "  warn: %s — %s\n", e.NoteID, e.Detail); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
