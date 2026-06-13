package cli

import (
	"fmt"

	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// MigrateTSVDeps is the dependency bundle for the migrate-from-tsv subcommand.
type MigrateTSVDeps struct {
	MigrateTSV *usecase.MigrateTSV
	UserID     string
}

// newMigrateTSVCmd builds the `flow worktime migrate-from-tsv` subcommand.
// It reads the legacy ~/.tmux/worktime.log TSV, maps every row to a Session
// in the SQLite store, and renames the TSV to .migrated-<ts> so the legacy
// adapter will not reload it.
//
// The migration is idempotent: UUIDv5 keys ensure re-running produces the
// same row IDs, so Upsert is a no-op on the database side for already-seen
// rows.
func newMigrateTSVCmd(deps MigrateTSVDeps) *cobra.Command {
	var tsvPath string
	var projectName string

	cmd := &cobra.Command{
		Use:   "migrate-from-tsv",
		Short: "Worktime-Sessions aus Legacy-TSV in SQLite migrieren",
		Long: `Liest die Legacy-Worktime-Datei (worktime.log) und speichert jede Zeile
als Session in der SQLite-Datenbank.

Die Migration ist idempotent: jede Zeile erhält eine stabile UUID (v5),
sodass erneutes Ausführen keine Duplikate erzeugt.

Nach erfolgreichem Import wird die TSV-Datei in worktime.log.migrated-<ts>
umbenannt, damit der alte Adapter sie nicht mehr lädt.

Fehlt die Datei unter --tsv, ist die Migration ein No-op (kein Fehler).`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.MigrateTSV == nil {
				return fmt.Errorf("migrate-from-tsv: use case not wired (composition-root bug)")
			}
			if tsvPath == "" {
				return fmt.Errorf("migrate-from-tsv: --tsv path is required")
			}
			res, err := deps.MigrateTSV.Run(deps.UserID, tsvPath, projectName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if res.ArchivedTo == "" {
				// TSV was absent — graceful no-op.
				fprintf(out, "TSV nicht gefunden unter %s — nichts zu migrieren.\n", tsvPath)
				return nil
			}
			fprintf(out, "Migriert: %d Session(s)\n", res.Inserted)
			if res.SkippedMalformed > 0 {
				fprintf(out, "Übersprungen (ungültige Zeilen): %d\n", res.SkippedMalformed)
			}
			fprintf(out, "Projekt: %s (%s)\n", res.DefaultProject.Name, res.DefaultProject.Slug)
			fprintf(out, "TSV archiviert nach: %s\n", res.ArchivedTo)
			return nil
		},
	}

	cmd.Flags().StringVar(&tsvPath, "tsv", "", "Pfad zur Legacy-TSV-Datei (z.B. ~/.tmux/worktime.log)")
	cmd.Flags().StringVar(&projectName, "project-name", "Allgemein",
		"Name des Ziel-Projekts (wird erstellt wenn nicht vorhanden)")
	return cmd
}
