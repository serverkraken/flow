package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func newImportLegacyCmd(deps Deps) *cobra.Command {
	var (
		dailyDir   string
		projectDir string
	)
	cmd := &cobra.Command{
		Use:   "import-legacy",
		Short: "Migrate notes from the old tmux notes/project-notes plugins",
		Long: "One-shot migration of legacy tmux-era notes: ~/notes/YYYY-MM-DD.md become " +
			"daily/YYYY-MM-DD, ~/.project-notes/<repo>-<hash>.md become " +
			"projects/<canonical-url>/_project. Existing target IDs are skipped — re-running " +
			"is safe.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			daily, project, err := resolveLegacyDirs(dailyDir, projectDir)
			if err != nil {
				return err
			}
			out, err := deps.ImportLegacy.Execute(cmd.Context(), usecase.ImportLegacyInput{
				DailyDir:   daily,
				ProjectDir: project,
			})
			if err != nil {
				return err
			}
			return printLegacyReport(cmd.OutOrStdout(), out)
		},
	}
	cmd.Flags().StringVar(&dailyDir, "daily-dir", "", "source for daily notes (default: $NOTES_DIR or ~/notes)")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "source for project notes (default: ~/.project-notes)")
	return cmd
}

// resolveLegacyDirs picks the source directories the migration reads from.
// $NOTES_DIR is intentionally NOT consulted: that variable now points at
// the destination notebook root, so falling back to it would have the
// importer read from the empty target instead of the actual legacy files.
// Defaults follow the old tmux notes / project-notes plugin layout —
// override with --daily-dir / --project-dir for unusual setups.
func resolveLegacyDirs(dailyDir, projectDir string) (string, string, error) {
	if dailyDir == "" || projectDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("home: %w", err)
		}
		if dailyDir == "" {
			dailyDir = filepath.Join(home, "notes")
		}
		if projectDir == "" {
			projectDir = filepath.Join(home, ".project-notes")
		}
	}
	return dailyDir, projectDir, nil
}

func printLegacyReport(w io.Writer, out usecase.ImportLegacyOutput) error {
	if _, err := fmt.Fprintf(w, "Migrated: %d\n", len(out.Migrated)); err != nil {
		return err
	}
	for _, id := range out.Migrated {
		if _, err := fmt.Fprintf(w, "  + %s\n", id); err != nil {
			return err
		}
	}
	if len(out.Skipped) > 0 {
		if _, err := fmt.Fprintf(w, "\nSkipped: %d\n", len(out.Skipped)); err != nil {
			return err
		}
		for _, s := range out.Skipped {
			if _, err := fmt.Fprintf(w, "  - %s: %s\n", s.Path, s.Reason); err != nil {
				return err
			}
		}
	}
	return nil
}
