package dayoffstsv

import (
	"bufio"
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
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/domain"
)

// Store reads and writes day-off entries to a primary TSV file with an
// optional read-only legacy-path fallback.
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
// Holds mu for the entire read-modify-write so two concurrent Add/Remove
// calls can't both read the same prior state and have the second
// overwrite the first. The read goes through readLocked so the
// legacy-path fallback applies — without that, the first Add against a
// user with only legacy data would silently mask all prior entries
// (the new primary file would contain just the one fresh row).
func (s *Store) Add(off domain.DayOff) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh := s.readLocked()
	merged := cloneMap(fresh)
	merged[off.Date.Format("2006-01-02")] = off
	return s.writeLocked(merged)
}

// Remove deletes the entry for date. Removing a non-existent entry is a
// no-op (no error).
func (s *Store) Remove(date time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh := s.readLocked()
	key := date.Format("2006-01-02")
	if _, ok := fresh[key]; !ok {
		return nil
	}
	merged := cloneMap(fresh)
	delete(merged, key)
	return s.writeLocked(merged)
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
func (s *Store) readLocked() map[string]domain.DayOff {
	if s.primed {
		return s.cache
	}
	out := readFile(s.path)
	if len(out) == 0 && s.legacyPath != "" {
		if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
			out = readFile(s.legacyPath)
		}
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

// readFile parses a TSV file. Missing or unreadable file → empty map.
// Per-line parse errors are tolerated silently.
func readFile(path string) map[string]domain.DayOff {
	out := map[string]domain.DayOff{}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		entry, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		out[entry.Date.Format("2006-01-02")] = entry
	}
	return out
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
