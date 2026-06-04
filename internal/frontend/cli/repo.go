// Package cli — `flow repo` subcommand tree (Plan C / M4).
//
// `flow repo note get`        — print the resolved RepoNote (empty if none)
// `flow repo note set [path]` — read content from --file/stdin/arg and save
// `flow repo note edit`       — open $EDITOR on the current note's content
package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// RepoDeps bundles the dependencies the `flow repo` command tree consumes.
// Constructed once in cmd/flow/main.go; the use case is also shared with
// the future flow-mcp server (Plan D).
type RepoDeps struct {
	UserID string
	Notes  *usecase.RepoNotes
	// EditorCmd defaults to $EDITOR or "vi". Tests override.
	EditorCmd func() string
}

// NewRepoCmd constructs the root `flow repo` command tree.
func NewRepoCmd(d RepoDeps) *cobra.Command {
	root := &cobra.Command{
		Use:          "repo",
		Short:        "Repo-Notes verwalten (CLAUDE-style note pro Repo, gesynct über alle Geräte)",
		SilenceUsage: true,
	}
	root.AddCommand(newRepoNoteCmd(d))
	return root
}

func newRepoNoteCmd(d RepoDeps) *cobra.Command {
	note := &cobra.Command{
		Use:          "note",
		Short:        "Repo-Note für das aktuelle Working-Directory",
		SilenceUsage: true,
	}
	note.AddCommand(newRepoNoteGetCmd(d))
	note.AddCommand(newRepoNoteSetCmd(d))
	note.AddCommand(newRepoNoteEditCmd(d))
	return note
}

func newRepoNoteGetCmd(d RepoDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "get",
		Short:        "Note des aktuellen Repos auf stdout drucken (leer wenn keine vorhanden)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("pwd: %w", err)
			}
			note, _, err := d.Notes.GetForPwd(d.UserID, pwd)
			if err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), note.Content)
			return nil
		},
	}
}

func newRepoNoteSetCmd(d RepoDeps) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "set [content]",
		Short: "Note für das aktuelle Repo schreiben",
		Long: `Quelle der Note:
  --file <path>  Datei lesen
  [content]      direkter Argument-String
  (kein Arg)     stdin lesen bis EOF`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readSetSource(cmd, args, file)
			if err != nil {
				return err
			}
			pwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("pwd: %w", err)
			}
			n, err := d.Notes.Save(d.UserID, pwd, content)
			if err != nil {
				return err
			}
			fprintf(cmd.ErrOrStderr(), "RepoNote gespeichert (%d Bytes, lokal — Server-Push läuft im Hintergrund)\n", len(n.Content))
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Pfad zur Datei mit dem Note-Inhalt")
	return cmd
}

func newRepoNoteEditCmd(d RepoDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "edit",
		Short:        "$EDITOR auf der aktuellen Note öffnen, beim Speichern persistieren",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("pwd: %w", err)
			}
			existing, _, err := d.Notes.GetForPwd(d.UserID, pwd)
			if err != nil {
				return err
			}
			updated, err := runEditor(d.editorCmd(), existing)
			if err != nil {
				return err
			}
			if updated == existing.Content {
				fprintln(cmd.ErrOrStderr(), "Keine Änderung — RepoNote unverändert.")
				return nil
			}
			n, err := d.Notes.Save(d.UserID, pwd, updated)
			if err != nil {
				return err
			}
			fprintf(cmd.ErrOrStderr(), "RepoNote gespeichert (%d Bytes)\n", len(n.Content))
			return nil
		},
	}
}

// editorCmd resolves the editor binary — explicit override, else $EDITOR,
// else "vi" as the POSIX default.
func (d RepoDeps) editorCmd() string {
	if d.EditorCmd != nil {
		return d.EditorCmd()
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

// readSetSource pulls the new content from --file, the arg, or stdin in
// that priority order.
func readSetSource(cmd *cobra.Command, args []string, file string) (string, error) {
	if file != "" {
		buf, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --file %q: %w", file, err)
		}
		return string(buf), nil
	}
	if len(args) == 1 {
		return args[0], nil
	}
	buf, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(buf), nil
}

// runEditor opens editorCmd on a tempfile pre-filled with the current
// content; returns the post-edit content. Editor's exit status must be 0.
func runEditor(editorCmd string, existing domain.RepoNote) (string, error) {
	tmp, err := os.CreateTemp("", "flow-repo-note-*.md")
	if err != nil {
		return "", fmt.Errorf("tempfile: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(existing.Content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("seed tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close tempfile: %w", err)
	}
	cmd := exec.Command(editorCmd, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q: %w", editorCmd, err)
	}
	buf, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("read edited tempfile: %w", err)
	}
	return string(buf), nil
}
