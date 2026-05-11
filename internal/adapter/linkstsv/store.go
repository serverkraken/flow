//go:build !windows

// linkstsv reaches for syscall.Flock / LOCK_EX / LOCK_UN to coordinate
// cross-process writes — POSIX-only constants. Until the cross-build
// matrix gains Windows, this package is silently skipped there. Review
// finding T9 (companion to flockstate's tag).

package linkstsv

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/adapter/textscan"
	"github.com/serverkraken/flow/internal/flockutil"
)

// Store reads and writes day-to-note attachments to a TSV file.
//
// Writers (Add/Remove) serialise on mu (in-process) AND syscall.Flock
// on a sibling .lock file (cross-process). Without the file lock, two
// flow instances racing on the same TSV — TUI + a goto.sh CLI helper,
// say — could each pass the dedup check and write a duplicate row, or
// race a read-modify-rewrite against an append and lose data.
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
	return s.withFileLock(func() error {
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
	})
}

// Remove detaches (date, noteID). Removing a non-existent pair is a no-op.
func (s *Store) Remove(date time.Time, noteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
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
	})
}

// withFileLock acquires an advisory POSIX lock on a sibling .lock file
// for cross-process serialisation. The lockfile is separate from the
// TSV itself because writeAll uses temp+rename and would otherwise
// pull the lock target out from under the holder.
//
// Failures are surfaced rather than silently degraded — review
// finding Q4. Pre-fix the function fell through to fn() on any
// MkdirAll / Open / Flock error, including non-transient ones like
// permission-denied or NFS lock-server outage. In-process `mu` still
// serialised, but cross-process writes raced and could corrupt the
// TSV. Now: surface the error so the user sees "links could not be
// updated" instead of silently broken state.
//
// Acquisition uses flockutil.Acquire (LOCK_EX|LOCK_NB + backoff up to
// flockutil.LockTimeout) so a stuck holder times out instead of wedging
// the caller indefinitely — review finding M3.
func (s *Store) withFileLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("links lockfile parent: %w", err)
	}
	lockPath := s.path + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("links lockfile open: %w", err)
	}
	defer lf.Close() //nolint:errcheck
	if err := flockutil.Acquire(int(lf.Fd()), flockutil.LockTimeout); err != nil {
		return fmt.Errorf("links flock: %w", err)
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
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
