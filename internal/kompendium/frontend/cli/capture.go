package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newCaptureCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "capture <text>",
		Short: "Append a timestamped bullet to today's daily without opening the editor",
		Long: "Quick capture: `kompendium capture \"the thing I just thought of\"` appends " +
			"`- HH:MM — the thing I just thought of` to today's daily note and exits. " +
			"Today's note is created on demand if it doesn't exist yet. Pair this with a " +
			"tmux popup or a shell alias for friction-free quick notes.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.Join(args, " ")
			out, err := deps.CaptureDaily.Execute(cmd.Context(), usecase.CaptureDailyInput{Text: text})
			if err != nil {
				return wrapAuthErr(err)
			}
			verb := "Captured"
			if out.Created {
				verb = "Created daily and captured"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s in %s\n  %s\n", verb, out.ID, out.Bullet)
			return err
		},
	}
}
