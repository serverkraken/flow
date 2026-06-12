package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// DocsDeps bundles dependencies for `flow docs` subcommands.
type DocsDeps struct {
	Docs   ports.DocumentStore
	UserID string
}

func newDocsCmd(deps DocsDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Markdown-Dokumente verwalten",
	}
	cmd.AddCommand(newDocsImportCmd(deps))
	return cmd
}

func newDocsImportCmd(deps DocsDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "import [dir]",
		Short: "Markdown-Verzeichnis idempotent importieren",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := os.Getenv("NOTES_DIR")
			if len(args) > 0 {
				dir = args[0]
			}
			if dir == "" {
				return errors.New("verzeichnis fehlt — Argument oder $NOTES_DIR angeben")
			}
			uc := &usecase.DocsImport{
				Docs:   deps.Docs,
				UserID: deps.UserID,
			}
			res, err := uc.Run(dir, func(path string) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", path)
			})
			if err != nil {
				if errors.Is(err, httpapi.ErrLoggedOut) {
					return errors.New("nicht eingeloggt — bitte `flow login` ausführen")
				}
				if errors.Is(err, httpapi.ErrNotConfigured) {
					return errors.New("server nicht konfiguriert — $FLOW_SERVER_URL setzen")
				}
				return err
			}
			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"importiert: %d neu, %d aktualisiert, %d unverändert, %d übersprungen\n",
				res.Created, res.Updated, res.Unchanged, res.Skipped,
			)
			return nil
		},
	}
}
