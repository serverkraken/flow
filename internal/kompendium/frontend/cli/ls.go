package cli

import (
	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newLsCmd(deps Deps) *cobra.Command {
	var (
		typeFilter    string
		projectFilter string
		currentRepo   string
		limit         int
		asJSON        bool
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List notes ordered by relevance for the read view",
		Long: "List notes filtered by type/project, with project notes for the current repo " +
			"promoted to the top tier when --current-repo is set.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := deps.ListNotes.Execute(cmd.Context(), usecase.ListNotesInput{
				Type:        domain.NoteType(typeFilter),
				Project:     projectFilter,
				CurrentRepo: domain.CanonicalURL(currentRepo),
				Limit:       limit,
			})
			if err != nil {
				return err
			}
			return printEntries(cmd.OutOrStdout(), entries, asJSON)
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "filter by type: daily, project, or free")
	cmd.Flags().StringVar(&projectFilter, "project", "", "filter by canonical project URL")
	cmd.Flags().StringVar(&currentRepo, "current-repo", "", "promote project notes for this canonical URL to the top tier")
	cmd.Flags().IntVar(&limit, "limit", 0, "max entries (0 = no limit)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of TSV")
	return cmd
}
