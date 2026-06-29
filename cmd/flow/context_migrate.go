package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func contextMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate the document corpus into the B3d type system",
	}
	cmd.AddCommand(migrateDocTypesCmd(), migrateMemoriesCmd())
	return cmd
}

func migrateDocTypesCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "doctypes",
		Short: "Rewrite legacy `agent` docs to spec/plan with slim paths",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := c.RedesignDocTypes(cmd.Context(), dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scanned %d agent docs, converted %d%s\n",
				rep.Scanned, rep.Converted, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without mutating")
	return cmd
}

// migrateMemoriesCmd is a stub; the real implementation is added in Task 8.
func migrateMemoriesCmd() *cobra.Command {
	return &cobra.Command{Use: "memories", Hidden: true}
}
