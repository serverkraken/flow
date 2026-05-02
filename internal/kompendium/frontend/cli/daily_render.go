package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newDailyRenderCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "daily-render <id>",
		Short: "Render a daily note with project aggregation header",
		Long: "Resolves a daily note plus the project notes that share its date and emits " +
			"the merged Markdown to stdout for downstream rendering.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := domain.ParseID(args[0])
			if err != nil {
				return err
			}
			out, err := deps.RenderDaily.Execute(cmd.Context(), usecase.RenderDailyInput{DailyID: id})
			if err != nil {
				return err
			}
			return renderDailyMarkdown(cmd.OutOrStdout(), out)
		},
	}
}

func renderDailyMarkdown(w io.Writer, r usecase.RenderDailyOutput) error {
	if len(r.ProjectsForDay) > 0 {
		if _, err := fmt.Fprintln(w, "## Projekt-Notizen heute"); err != nil {
			return err
		}
		for _, p := range r.ProjectsForDay {
			title := p.Meta.Title
			if title == "" {
				title = p.ID.String()
			}
			if _, err := fmt.Fprintf(w, "- [[%s]] – %s\n", p.ID, title); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "\n---"); err != nil {
			return err
		}
	}
	if len(r.Daily.Body) > 0 {
		if _, err := w.Write(r.Daily.Body); err != nil {
			return err
		}
	}
	return nil
}
