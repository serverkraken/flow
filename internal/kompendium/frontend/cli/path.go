package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func newPathCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path <id>",
		Short: "Print the absolute filesystem path of a note ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := domain.ParseID(args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), deps.Store.Path(id))
			return err
		},
	}
}
