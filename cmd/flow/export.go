package main

import (
	"os"

	"github.com/spf13/cobra"
)

func exportCmd() *cobra.Command {
	var from, to, format, project string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "export worktime (per project) for a range as csv|json|md to stdout",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// --project takes a slug for UX; the API filters by project id.
			projectID := ""
			if project != "" {
				var err error
				projectID, err = resolveSlug(cmd.Context(), c, project)
				if err != nil {
					return err
				}
			}
			b, err := c.Export(cmd.Context(), from, to, format, projectID)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(b)
			return err
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&to, "to", "", "end date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&format, "format", "csv", "csv|json|md")
	cmd.Flags().StringVar(&project, "project", "", "optional project slug filter")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
