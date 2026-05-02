package linkstsv

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store reads and writes day-to-note attachments to a TSV file.
type Store struct {
	path string
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
	existing, err := s.ListByDate(date)
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
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = fmt.Fprintf(f, "%s\t%s\n", date.Format("2006-01-02"), noteID)
	return err
}

// Remove detaches (date, noteID). Removing a non-existent pair is a no-op.
func (s *Store) Remove(date time.Time, noteID string) error {
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
	sc := bufio.NewScanner(f)
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
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, l := range links {
		if _, werr := fmt.Fprintf(f, "%s\t%s\n", l.Date.Format("2006-01-02"), l.NoteID); werr != nil {
			f.Close() //nolint:errcheck
			return werr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
