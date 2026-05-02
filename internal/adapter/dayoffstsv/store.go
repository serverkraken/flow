package dayoffstsv

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
func (s *Store) Add(off domain.DayOff) error {
	fresh := readFile(s.path)
	fresh[off.Date.Format("2006-01-02")] = off
	return s.write(fresh)
}

// Remove deletes the entry for date. Removing a non-existent entry is a
// no-op (no error).
func (s *Store) Remove(date time.Time) error {
	fresh := readFile(s.path)
	key := date.Format("2006-01-02")
	if _, ok := fresh[key]; !ok {
		return nil
	}
	delete(fresh, key)
	return s.write(fresh)
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

func (s *Store) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.primed = false
	s.mu.Unlock()
}

func (s *Store) write(m map[string]domain.DayOff) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if werr := writeBody(f, m, keys); werr != nil {
		f.Close() //nolint:errcheck
		return werr
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func writeBody(f *os.File, m map[string]domain.DayOff, keys []string) error {
	if _, err := fmt.Fprintln(f, "# worktime day-offs — TSV: date\\tkind\\tlabel[\\thours]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "# kinds: holiday | vacation | sick"); err != nil {
		return err
	}
	for _, k := range keys {
		d := m[k]
		row := fmt.Sprintf("%s\t%s\t%s",
			d.Date.Format("2006-01-02"), string(d.Kind), d.Label)
		if d.Target > 0 {
			row += fmt.Sprintf("\t%g", d.Target.Hours())
		}
		if _, err := fmt.Fprintln(f, row); err != nil {
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
