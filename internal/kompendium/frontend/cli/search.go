package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newSearchCmd(deps Deps) *cobra.Command {
	var (
		typeFilter    string
		projectFilter string
		order         string
		limit         int
		asJSON        bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across the notebook",
		Long: "Search the FTS5 index. Default order is relevance (bm25); use --order=recent " +
			"to sort by mtime instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ord, err := parseSearchOrder(order)
			if err != nil {
				return err
			}
			results, err := deps.SearchNotes.Execute(cmd.Context(), usecase.SearchNotesInput{
				Text:    args[0],
				Type:    domain.NoteType(typeFilter),
				Project: projectFilter,
				Order:   ord,
				Limit:   limit,
			})
			if err != nil {
				return err
			}
			return printSearchResults(cmd.OutOrStdout(), results, asJSON)
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "filter by type: daily, project, or free")
	cmd.Flags().StringVar(&projectFilter, "project", "", "filter by canonical project URL")
	cmd.Flags().StringVar(&order, "order", "relevance", "order by: relevance | recent")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results (0 = no limit)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of TSV")
	return cmd
}

func parseSearchOrder(s string) (domain.SearchOrder, error) {
	switch s {
	case "", "relevance":
		return domain.OrderRelevance, nil
	case "recent":
		return domain.OrderRecent, nil
	}
	return 0, fmt.Errorf("invalid --order %q (want relevance or recent)", s)
}
