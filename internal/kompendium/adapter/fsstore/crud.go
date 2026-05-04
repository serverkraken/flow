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
// sync would then propagate the corruption to every other machine. The
// parent directory is fsync'd after rename: POSIX permits the directory
// entry update itself to roll back on crash even when the file's data
// is durable, which would resurrect the pre-rename name pointing at a
// removed inode (or lose the new file entirely). Mirrors the discipline
// in flow's own atomicfile.WriteFile (see commit 9d515f1).
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
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir %q: %w", dir, err)
	}
	return nil
}

// syncDir fsync's the directory so the prior rename becomes durable.
// Some filesystems return an error from Sync on a directory FD (rare
// network FSes); that error is propagated rather than swallowed.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
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
