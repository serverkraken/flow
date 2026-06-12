package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"

	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// EditNote opens an existing note in the editor via a tempfile. The
// note body is serialised to a tempfile, the editor opens it, and on
// exit the diff is checked: if the content is unchanged the tempfile
// is deleted without a Put; if it changed the new content is parsed
// and written back through the store.
//
// On version conflict (ErrDocumentVersionConflict from Put) or frontmatter
// parse failure, the error message includes the tempfile path so the user
// can recover the edit.
type EditNote struct {
	Store  kompports.NoteStore
	Editor kompports.Editor
}

// Execute implements the tempfile-edit flow for a single note ID.
func (u *EditNote) Execute(ctx context.Context, id kompdomain.ID) error {
	note, err := u.Store.Get(ctx, id)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "flow-note-*.md")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	path := tmp.Name()

	raw := kompdomain.RenderNote(note)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close tempfile: %w", err)
	}

	if err := u.Editor.Edit(ctx, path); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("edit: %w", err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tempfile: %w", err)
	}

	if string(edited) == string(raw) {
		_ = os.Remove(path)
		return nil
	}

	updated, err := kompdomain.ParseNote(id, edited)
	if err != nil {
		return fmt.Errorf("frontmatter kaputt — Bearbeitung liegt in %s: %w", path, err)
	}

	if err := u.Store.Put(ctx, updated); err != nil {
		if errors.Is(err, ports.ErrDocumentVersionConflict) {
			return fmt.Errorf("note wurde parallel geändert — Bearbeitung liegt in %s; neu öffnen und zusammenführen: %w", path, err)
		}
		return err
	}

	_ = os.Remove(path)
	return nil
}
