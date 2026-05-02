package cli

import "github.com/spf13/cobra"

// newTodayCmd is a typing-saver alias for `kompendium new daily`. It
// resolves the same use case under a shorter, more habitual verb.
func newTodayCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Open today's daily note (alias for `new daily`)",
		Long: "Resolves to the same code path as `kompendium new daily`: creates today's daily " +
			"note if missing, then opens it in the editor. Saves typing for what is the most " +
			"frequent kompendium command in daily use.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := deps.CreateDaily.Execute(cmd.Context())
			if err != nil {
				return err
			}
			return printCreateOutput(cmd.OutOrStdout(), out.ID, out.Created, out.Path)
		},
	}
}
