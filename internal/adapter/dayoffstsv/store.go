//go:build !windows

// dayoffstsv reaches for syscall.Flock / LOCK_EX / LOCK_UN to coordinate
// cross-process writes — POSIX-only constants. Until the cross-build
// matrix gains Windows, this package is silently skipped there (same
// pattern as linkstsv and flockstate).

package dayoffstsv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/adapter/textscan"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/flockutil"
)

// Store reads and writes day-off entries to a primary TSV file with an
// optional read-only legacy-path fallback.
//
// Writers (Add/Remove) serialise on mu (in-process) AND on a POSIX
// advisory file lock on a sibling .lock file (cross-process). Without
// the file lock, the CLI `flow worktime dayoff add` racing the TUI's
// SyncGermanHolidays could each pass the read step against identical
// state, and the second atomicfile.WriteFile rename would silently
// discard the first writer's row. Matches the pattern in linkstsv.
type Store struct {
	path       string
	legacyPath string

	mu     sync.RWMutex
	cache  map[string]domain.DayOff
	primed bool
}

// New constructs a Store that writes to primary and falls back to
// legacy on read when primary does not exist. Pass legacy = "" to
// disable the fallback.
func New(primary, legacy string) *Store {
	return &Store{path: primary, legacyPath: legacy}
}

// List returns entries with from <= date <= to (sorted ascending).
// A zero from/to disables the bound on that side.
func (s *Store) List(from, to time.Time) []domain.DayOff {
	all := s.readCached()
	out := make([]domain.DayOff, 0, len(all))
	for _, d := range all {
		if !from.IsZero() && d.Date.Before(from) {
			continue
		}
		if !to.IsZero() && d.Date.After(to) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// Lookup returns the entry for date and a found flag.
func (s *Store) Lookup(date time.Time) (domain.DayOff, bool) {
	d, ok := s.readCached()[date.Format("2006-01-02")]
	return d, ok
}

// Add inserts or replaces the entry for off.Date. Validation (kind,
// label) is the use-case's job; the adapter only persists.
//
// Holds mu (in-process) and a POSIX advisory file lock on a sibling
// .lock (cross-process) for the entire read-modify-write so two
// concurrent Add/Remove calls — within or across processes — can't
// both read the same prior state and have the second overwrite the
// first. The cache is invalidated under the lock so a second writer
// in the same process sees rows another process landed since the
// first writer's commit. The read goes through readLocked so the
// legacy-path fallback applies — without that, the first Add against a
// user with only legacy data would silently mask all prior entries
// (the new primary file would contain just the one fresh row).
func (s *Store) Add(off domain.DayOff) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		s.primed = false
		fresh := s.readLocked()
		merged := cloneMap(fresh)
		merged[off.Date.Format("2006-01-02")] = off
		return s.writeLocked(merged)
	})
}

// AddBatch inserts or replaces every entry in offs as one atomic
// read-modify-write — the same lock + cache discipline as Add, but the
// final atomicfile.WriteFile carries all rows at once. Either every
// entry lands or none does. An empty slice is a no-op (no file touch).
//
// Use-case caller is DayOffWriter.AddRange: a multi-day vacation
// booking must be all-or-nothing because partial-progress would leave
// the user with orphaned days and no signal which ones.
func (s *Store) AddBatch(offs []domain.DayOff) error {
	if len(offs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		s.primed = false
		fresh := s.readLocked()
		merged := cloneMap(fresh)
		for _, off := range offs {
			merged[off.Date.Format("2006-01-02")] = off
		}
		return s.writeLocked(merged)
	})
}

// Remove deletes the entry for date. Removing a non-existent entry is a
// no-op (no error).
func (s *Store) Remove(date time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		s.primed = false
		fresh := s.readLocked()
		key := date.Format("2006-01-02")
		if _, ok := fresh[key]; !ok {
			return nil
		}
		merged := cloneMap(fresh)
		delete(merged, key)
		return s.writeLocked(merged)
	})
}

// withFileLock acquires an advisory POSIX lock on a sibling .lock file
// for cross-process serialisation. The lockfile is separate from the
// TSV itself because writeLocked uses temp+rename and would otherwise
// pull the lock target out from under the holder.
//
// Failures surface rather than degrade silently — without that, two
// processes whose Open or Flock fails (NFS lock-server outage,
// permission-denied) would both fall through to fn() and race anyway.
// Pattern mirrors linkstsv.withFileLock (review finding Q4).
//
// Acquisition uses flockutil.Acquire (LOCK_EX|LOCK_NB + backoff up to
// flockutil.LockTimeout) so a stuck holder times out instead of wedging
// the caller indefinitely — review finding M3.
func (s *Store) withFileLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("dayoffs lockfile parent: %w", err)
	}
	lockPath := s.path + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("dayoffs lockfile open: %w", err)
	}
	defer lf.Close() //nolint:errcheck
	if err := flockutil.Acquire(int(lf.Fd()), flockutil.LockTimeout); err != nil {
		return fmt.Errorf("dayoffs flock: %w", err)
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func (s *Store) readCached() map[string]domain.DayOff {
	s.mu.RLock()
	if s.primed {
		m := s.cache
		s.mu.RUnlock()
		return m
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

// readLocked returns the cached map, priming it from disk (with legacy
// fallback) on first call. Caller must hold mu.
//
// Legacy fallback fires whenever the primary path produces nil — both
// "file does not exist" and "file unreadable / parse failed" — rather
// than only os.ErrNotExist. Otherwise a corrupted primary masked all
// legacy data and silently shipped writes to the broken file.
func (s *Store) readLocked() map[string]domain.DayOff {
	if s.primed {
		return s.cache
	}
	out, err := readFile(s.path)
	if (err != nil || out == nil) && s.legacyPath != "" {
		if legacy, lerr := readFile(s.legacyPath); lerr == nil && legacy != nil {
			out = legacy
		}
	}
	if out == nil {
		out = map[string]domain.DayOff{}
	}
	s.cache = out
	s.primed = true
	return out
}

func cloneMap(in map[string]domain.DayOff) map[string]domain.DayOff {
	out := make(map[string]domain.DayOff, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// writeLocked persists m and refreshes the cache to match. Caller must
// hold mu. The cache is set to m (not nil) so subsequent List/Lookup
// calls — fired on every TUI tick — don't trigger a disk reread.
func (s *Store) writeLocked(m map[string]domain.DayOff) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	if err := writeBody(&buf, m, keys); err != nil {
		return err
	}
	if err := atomicfile.WriteFile(s.path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	s.cache = m
	s.primed = true
	return nil
}

func writeBody(w io.Writer, m map[string]domain.DayOff, keys []string) error {
	if _, err := fmt.Fprintln(w, "# worktime day-offs — TSV: date<TAB>kind<TAB>label[<TAB>hours]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# kinds: holiday | vacation | sick"); err != nil {
		return err
	}
	for _, k := range keys {
		d := m[k]
		row := fmt.Sprintf("%s\t%s\t%s",
			d.Date.Format("2006-01-02"), string(d.Kind), d.Label)
		if d.Target > 0 {
			row += fmt.Sprintf("\t%g", d.Target.Hours())
		}
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	return nil
}

// readFile parses a TSV file.
//
// Returns:
//   - (nil, nil)  → file does not exist or path is empty
//   - (map, nil)  → file read OK (map may legitimately be empty)
//   - (nil, err)  → I/O failure (permission denied, scan error, etc.)
//
// The (nil, nil) "file missing" case is distinct from (nil, err) so
// readLocked can fall back to the legacy path on any non-existence
// AND on any I/O failure.
func readFile(path string) (map[string]domain.DayOff, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	out := map[string]domain.DayOff{}
	sc := textscan.New(f)
	for sc.Scan() {
		entry, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		out[entry.Date.Format("2006-01-02")] = entry
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseLine(raw string) (domain.DayOff, bool) {
	line := strings.TrimRight(raw, "\r\n")
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return domain.DayOff{}, false
	}
	parts := strings.Split(line, "\t")
	if len(parts) < 2 {
		return domain.DayOff{}, false
	}
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
	if err != nil {
		return domain.DayOff{}, false
	}
	entry := domain.DayOff{Date: d}

	if len(parts) == 2 {
		entry.Kind = domain.KindHoliday
		entry.Label = strings.TrimSpace(parts[1])
		return entry, true
	}

	second := strings.TrimSpace(parts[1])
	if k, ok := domain.ParseKind(second); ok {
		entry.Kind = k
		entry.Label = strings.TrimSpace(parts[2])
		if len(parts) >= 4 {
			entry.Target = parseHours(parts[3])
		}
		return entry, true
	}

	// Legacy: second column is the label, optional third column is hours.
	entry.Kind = domain.KindHoliday
	entry.Label = second
	if len(parts) >= 3 {
		entry.Target = parseHours(parts[2])
	}
	return entry, true
}

func parseHours(s string) time.Duration {
	hours, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || hours < 0 {
		return 0
	}
	return time.Duration(hours * float64(time.Hour))
}
