package fsstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Get implements ports.NoteStore.
func (s *Store) Get(_ context.Context, id domain.ID) (domain.Note, error) {
	p := s.Path(id)
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.Note{}, ports.ErrNoteNotFound
		}
		return domain.Note{}, fmt.Errorf("read %q: %w", p, err)
	}
	fm, body, err := domain.ParseFrontmatter(raw)
	if err != nil {
		return domain.Note{}, fmt.Errorf("parse frontmatter of %q: %w", p, err)
	}
	return domain.Note{ID: id, Meta: fm, Body: body}, nil
}

// Put implements ports.NoteStore. Intermediate directories are created on
// demand so callers don't have to mkdir manually.
//
// Writes go via a sibling tmp file followed by os.Rename so a crash mid-
// write can never leave a half-written .md in the notebook — git would
// otherwise happily commit the partial file on the next snapshot, and
// sync would then propagate the corruption to every other machine.
func (s *Store) Put(_ context.Context, n domain.Note) error {
	p := s.Path(n.ID)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".kompendium-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	data := n.Meta.Serialize(n.Body)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp %q: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsync temp %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp %q: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		cleanup()
		return fmt.Errorf("rename %q → %q: %w", tmpPath, p, err)
	}
	return nil
}

// Delete implements ports.NoteStore.
func (s *Store) Delete(_ context.Context, id domain.ID) error {
	p := s.Path(id)
	if err := os.Remove(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ports.ErrNoteNotFound
		}
		return fmt.Errorf("remove %q: %w", p, err)
	}
	return nil
}

// Exists implements ports.NoteStore.
func (s *Store) Exists(_ context.Context, id domain.ID) (bool, error) {
	p := s.Path(id)
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %q: %w", p, err)
}
