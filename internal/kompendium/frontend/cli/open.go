package cli

import (
	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newOpenCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "open <id>",
		Short: "Open an existing note in the editor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := domain.ParseID(args[0])
			if err != nil {
				return err
			}
			return wrapAuthErr(deps.Open.Execute(cmd.Context(), usecase.OpenInput{ID: id}))
		},
	}
}
