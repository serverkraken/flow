package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func newPathCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path <id>",
		Short: "Print the absolute filesystem path of a note ID (local notebook only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := domain.ParseID(args[0])
			if err != nil {
				return err
			}
			if deps.Rooter == nil {
				return fmt.Errorf("path command requires a local notebook (not available with server store)")
			}
			p := filepath.Join(deps.Rooter.Root(), filepath.FromSlash(id.Path()))
			_, err = fmt.Fprintln(cmd.OutOrStdout(), p)
			return err
		},
	}
}
