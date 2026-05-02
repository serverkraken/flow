package fsstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// frontmatterReadCap caps how many bytes List reads per file when
// extracting frontmatter — well over typical YAML headers but small
// enough to keep `kompendium ls` fast on big notebooks. Beyond this
// the reader gives up parsing and skips the file rather than load the
// whole body just to derive metadata.
const frontmatterReadCap = 8 * 1024

// List implements ports.NoteStore. Files whose path is not a valid note ID
// or whose contents lack a parseable frontmatter are silently skipped — they
// are not part of the notebook's namespace as far as the store is concerned.
func (s *Store) List(_ context.Context, filter ports.ListFilter) ([]ports.NoteEntry, error) {
	entries, err := s.walk(filter)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Mtime.After(entries[j].Mtime)
	})
	if filter.Limit > 0 && len(entries) > filter.Limit {
		entries = entries[:filter.Limit]
	}
	return entries, nil
}

func (s *Store) walk(filter ports.ListFilter) ([]ports.NoteEntry, error) {
	var entries []ports.NoteEntry
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		entry, ok, err := s.entryFor(path, d, filter)
		if err != nil {
			return err
		}
		if ok {
			entries = append(entries, entry)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %q: %w", s.root, err)
	}
	return entries, nil
}

func (s *Store) entryFor(path string, d fs.DirEntry, filter ports.ListFilter) (ports.NoteEntry, bool, error) {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return ports.NoteEntry{}, false, fmt.Errorf("rel %q under %q: %w", path, s.root, err)
	}
	idStr := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
	id, err := domain.ParseID(idStr)
	if err != nil {
		return ports.NoteEntry{}, false, nil
	}
	fm, ok, err := readFrontmatterCapped(path)
	if err != nil {
		return ports.NoteEntry{}, false, fmt.Errorf("read header %q: %w", path, err)
	}
	if !ok {
		return ports.NoteEntry{}, false, nil
	}
	if filter.Type != "" && fm.Type != filter.Type {
		return ports.NoteEntry{}, false, nil
	}
	if filter.Project != "" && fm.Project != filter.Project {
		return ports.NoteEntry{}, false, nil
	}
	info, err := d.Info()
	if err != nil {
		return ports.NoteEntry{}, false, fmt.Errorf("stat %q: %w", path, err)
	}
	return ports.NoteEntry{ID: id, Meta: fm, Mtime: info.ModTime()}, true, nil
}

// readFrontmatterCapped reads at most frontmatterReadCap bytes of the
// note file and parses the frontmatter from that prefix. A note whose
// frontmatter doesn't fit in the cap is treated as un-listable (ok=false)
// — that is several orders of magnitude beyond any realistic note YAML
// header, so it's a defensive cap, not a real-world constraint.
func readFrontmatterCapped(path string) (domain.Frontmatter, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.Frontmatter{}, false, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, frontmatterReadCap)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return domain.Frontmatter{}, false, err
	}
	prefix := buf[:n]

	// Synthesise a valid closing delimiter when the cap cuts mid-frontmatter
	// so ParseFrontmatter at least produces a partial Frontmatter rather
	// than ErrMalformedFrontmatter. The result is reported as un-listable
	// so callers can decide.
	if !bytes.Contains(prefix, []byte("\n---\n")) && !bytes.HasSuffix(prefix, []byte("\n---")) {
		// No closing marker in the read window — give up rather than
		// pretend the frontmatter parsed.
		fm, _, err := domain.ParseFrontmatter(prefix)
		if err != nil {
			return domain.Frontmatter{}, false, nil
		}
		return fm, true, nil
	}

	fm, _, err := domain.ParseFrontmatter(prefix)
	if err != nil {
		return domain.Frontmatter{}, false, nil
	}
	return fm, true, nil
}
