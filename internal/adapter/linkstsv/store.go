package linkstsv

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/adapter/textscan"
)

// Store reads and writes day-to-note attachments to a TSV file.
//
// Writers (Add/Remove) serialise on mu so two concurrent Add calls for
// the same (date, noteID) can't both pass the dedup check, append, and
// produce a duplicated row.
type Store struct {
	path string

	mu sync.Mutex
}

// New constructs a Store backed by path. The file is created on first
// successful Add.
func New(path string) *Store {
	return &Store{path: path}
}

// ListByDate returns the note IDs attached to date in insertion order.
// Empty slice when none, nil when the file does not exist.
func (s *Store) ListByDate(date time.Time) ([]string, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}
	key := date.Format("2006-01-02")
	var out []string
	for _, l := range all {
		if l.Date.Format("2006-01-02") == key {
			out = append(out, l.NoteID)
		}
	}
	return out, nil
}

// Add appends (date, noteID). Idempotent: if the pair already exists,
// returns nil without touching the file.
func (s *Store) Add(date time.Time, noteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.listByDateLocked(date)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if id == noteID {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf []byte
	buf = fmt.Appendf(buf, "%s\t%s\n", date.Format("2006-01-02"), noteID)
	return atomicfile.Append(s.path, buf, 0o644)
}

// Remove detaches (date, noteID). Removing a non-existent pair is a no-op.
func (s *Store) Remove(date time.Time, noteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	key := date.Format("2006-01-02")
	kept := make([]link, 0, len(all))
	removed := false
	for _, l := range all {
		if !removed && l.Date.Format("2006-01-02") == key && l.NoteID == noteID {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return nil
	}
	return s.writeAll(kept)
}

func (s *Store) listByDateLocked(date time.Time) ([]string, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}
	key := date.Format("2006-01-02")
	var out []string
	for _, l := range all {
		if l.Date.Format("2006-01-02") == key {
			out = append(out, l.NoteID)
		}
	}
	return out, nil
}

type link struct {
	Date   time.Time
	NoteID string
}

func (s *Store) readAll() ([]link, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var links []link
	sc := textscan.New(f)
	for sc.Scan() {
		l, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		links = append(links, l)
	}
	return links, sc.Err()
}

func parseLine(raw string) (link, bool) {
	line := strings.TrimRight(raw, "\r\n")
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return link{}, false
	}
	parts := strings.SplitN(line, "\t", 2)
	if len(parts) != 2 {
		return link{}, false
	}
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
	if err != nil {
		return link{}, false
	}
	noteID := strings.TrimSpace(parts[1])
	if noteID == "" {
		return link{}, false
	}
	return link{Date: date, NoteID: noteID}, true
}

func (s *Store) writeAll(links []link) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, l := range links {
		if _, werr := fmt.Fprintf(&buf, "%s\t%s\n", l.Date.Format("2006-01-02"), l.NoteID); werr != nil {
			return werr
		}
	}
	return atomicfile.WriteFile(s.path, buf.Bytes(), 0o644)
}
