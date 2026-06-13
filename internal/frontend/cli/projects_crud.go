package cli

import (
	"fmt"
	"regexp"
	"text/tabwriter"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// ProjectsCRUDDeps is the dependency bundle for the CRUD subcommands of
// `flow projects`. Injected by the composition root; the TUI launcher
// (Screen) lives in ProjectsDeps and is not repeated here.
type ProjectsCRUDDeps struct {
	Projects *usecase.Projects
	UserID   string
}

// uuidPattern matches the canonical 8-4-4-4-12 hex UUID form.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// resolveProjectRef resolves a <slug-or-id> argument to a domain.Project.
// If ref matches the UUID pattern it is looked up by ID; otherwise by slug.
// Returns ports.ErrProjectNotFound when no matching project exists.
func resolveProjectRef(store ports.ProjectStore, userID, ref string) (domain.Project, error) {
	if uuidPattern.MatchString(ref) {
		return store.GetByID(userID, ref)
	}
	return store.GetBySlug(userID, ref)
}

// newProjectsListCmd implements `flow projects list [--archived]`.
//
// SESSIONS column shows "–" (en-dash) — a session-count query requires a
// dedicated port method not in Task 13 scope.
func newProjectsListCmd(deps ProjectsCRUDDeps) *cobra.Command {
	var archived bool
	cmd := &cobra.Command{
		Use:          "list",
		Aliases:      []string{"ls"},
		Short:        "Projekte auflisten (default: aktive; --archived: auch archivierte)",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var ps []domain.Project
			var err error
			if archived {
				ps, err = deps.Projects.ListAll(deps.UserID)
			} else {
				ps, err = deps.Projects.ListActive(deps.UserID)
			}
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "SLUG\tNAME\tLAST_USED\tSESSIONS\tARCHIVED")
			for _, p := range ps {
				lastUsed := "–"
				if !p.LastUsedAt.IsZero() {
					lastUsed = p.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
				}
				archivedAt := ""
				if p.ArchivedAt != nil {
					archivedAt = p.ArchivedAt.UTC().Format("2006-01-02T15:04:05Z")
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t–\t%s\n",
					p.Slug, p.Name, lastUsed, archivedAt)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "archivierte Projekte ebenfalls anzeigen")
	return cmd
}

// newProjectsCreateCmd implements `flow projects create <name>`.
func newProjectsCreateCmd(deps ProjectsCRUDDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "create <name>",
		Short:        "Neues Worktime-Projekt anlegen",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := deps.Projects.Create(deps.UserID, args[0])
			if err != nil {
				return err
			}
			fprintf(cmd.OutOrStdout(), "%s\n", p.Slug)
			return nil
		},
	}
}

// newProjectsRenameCmd implements `flow projects rename <slug-or-id> <new-name>`.
func newProjectsRenameCmd(deps ProjectsCRUDDeps, store ports.ProjectStore) *cobra.Command {
	return &cobra.Command{
		Use:          "rename <slug-or-id> <new-name>",
		Short:        "Projektnamen aendern (Slug bleibt stabil)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := resolveProjectRef(store, deps.UserID, args[0])
			if err != nil {
				return fmt.Errorf("project %q not found: %w", args[0], err)
			}
			if err := deps.Projects.Rename(deps.UserID, proj.ID, args[1]); err != nil {
				return err
			}
			fprintf(cmd.ErrOrStderr(), "renamed %s -> %q\n", proj.Slug, args[1])
			return nil
		},
	}
}

// newProjectsArchiveCmd implements `flow projects archive <slug-or-id>`.
func newProjectsArchiveCmd(deps ProjectsCRUDDeps, store ports.ProjectStore) *cobra.Command {
	return &cobra.Command{
		Use:          "archive <slug-or-id>",
		Short:        "Projekt archivieren (Soft-Delete; Sessions bleiben erhalten)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := resolveProjectRef(store, deps.UserID, args[0])
			if err != nil {
				return fmt.Errorf("project %q not found: %w", args[0], err)
			}
			if err := deps.Projects.Archive(deps.UserID, proj.ID); err != nil {
				return err
			}
			fprintf(cmd.ErrOrStderr(), "archived %s\n", proj.Slug)
			return nil
		},
	}
}
