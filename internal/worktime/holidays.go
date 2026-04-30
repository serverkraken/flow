// Package worktime — day-off layer.
//
// A "day off" is any date where the daily work target is reduced (typically
// to zero). Three kinds are tracked uniformly:
//
//   - holiday  : public/statutory holidays (e.g. Karfreitag in NRW)
//   - vacation : user-managed holidays
//   - sick     : sick days
//
// Storage: ~/.tmux/worktime-dayoffs.tsv (override via WORKTIME_DAYOFFS_FILE)
// Format: YYYY-MM-DD \t kind \t label [\t target_hours]
//
// All callers go through this file. The legacy filename worktime-holidays.tsv
// is read as a fallback so existing setups continue to work.
package worktime

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
)

// Kind classifies a day off.
type Kind string

// Day-off categories. Persisted as the literal string in the second column
// of worktime-dayoffs.tsv, so renaming requires a migration.
const (
	KindHoliday  Kind = "holiday"  // gesetzlicher Feiertag
	KindVacation Kind = "vacation" // Urlaub
	KindSick     Kind = "sick"     // Krankheit
)

// AllKinds enumerates valid kinds (used by UI cycling and CLI validation).
var AllKinds = []Kind{KindHoliday, KindVacation, KindSick}

// ParseKind tolerates German UI strings ("Urlaub", "Krank", "Feiertag") and
// short forms ("v", "s", "h"). Returns ("", false) on unknown input.
func ParseKind(s string) (Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "h", "holiday", "feiertag":
		return KindHoliday, true
	case "v", "vacation", "urlaub":
		return KindVacation, true
	case "s", "sick", "krank", "krankheit":
		return KindSick, true
	}
	return "", false
}

// LabelDe renders the German label for a kind ("Feiertag", "Urlaub", "Krank").
func (k Kind) LabelDe() string {
	switch k {
	case KindHoliday:
		return "Feiertag"
	case KindVacation:
		return "Urlaub"
	case KindSick:
		return "Krank"
	}
	return string(k)
}

// DayOff is one named day-off entry with an optional target override.
// Target == 0 means "full day off"; > 0 reduces the day's target (half-day);
// -1 means "no override" (rare, kept for forward compat).
type DayOff struct {
	Date   time.Time
	Kind   Kind
	Label  string
	Target time.Duration
}

// Holiday is the legacy alias kept so existing call sites compile during the
// transition. New code should use DayOff.
type Holiday = DayOff

// dayoffsPath returns the active path; WORKTIME_DAYOFFS_FILE wins, then
// WORKTIME_HOLIDAYS_FILE (back-compat), then ~/.tmux/worktime-dayoffs.tsv.
func dayoffsPath() string {
	if v := os.Getenv("WORKTIME_DAYOFFS_FILE"); v != "" {
		return v
	}
	if v := os.Getenv("WORKTIME_HOLIDAYS_FILE"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tmux", "worktime-dayoffs.tsv")
}

// legacyDayoffsPath is read as a fallback if dayoffsPath() doesn't exist.
// This keeps users with the old filename from losing their data on upgrade.
func legacyDayoffsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tmux", "worktime-holidays.tsv")
}

var (
	dayoffCacheMu   sync.RWMutex
	dayoffCacheMap  map[string]DayOff
	dayoffCachePath string
)

// invalidateDayOffCache resets the cache. Tests use this when switching HOME.
// Production code calls it after a successful Add/Remove to keep TargetFor
// fresh without restarting the process.
func invalidateDayOffCache() {
	dayoffCacheMu.Lock()
	dayoffCacheMap = nil
	dayoffCachePath = ""
	dayoffCacheMu.Unlock()
}

// ResetCachesForTesting is the test-only entry point for clearing process-wide
// caches between tests. The exported name is required because tests live in
// the worktime_test package and can't reach the unexported invalidator.
// Do not call this from production code.
func ResetCachesForTesting() { invalidateDayOffCache() }

// loadDayOffs reads the file and returns a map keyed by YYYY-MM-DD.
// Missing/unreadable file → empty map. Process-wide cache keyed by path.
func loadDayOffs() map[string]DayOff {
	path := dayoffsPath()
	dayoffCacheMu.RLock()
	if dayoffCacheMap != nil && dayoffCachePath == path {
		m := dayoffCacheMap
		dayoffCacheMu.RUnlock()
		return m
	}
	dayoffCacheMu.RUnlock()

	dayoffCacheMu.Lock()
	defer dayoffCacheMu.Unlock()
	if dayoffCacheMap != nil && dayoffCachePath == path {
		return dayoffCacheMap
	}

	out := readDayOffsFromFile(path)
	if len(out) == 0 {
		// Legacy fallback only if the active file is missing AND we're not
		// pointing at a custom path via the env var.
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) && os.Getenv("WORKTIME_DAYOFFS_FILE") == "" {
			out = readDayOffsFromFile(legacyDayoffsPath())
		}
	}
	dayoffCacheMap = out
	dayoffCachePath = path
	return out
}

func readDayOffsFromFile(path string) map[string]DayOff {
	out := map[string]DayOff{}
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
		raw := strings.TrimRight(sc.Text(), "\r\n")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		parts := strings.Split(raw, "\t")
		if len(parts) < 2 {
			continue
		}
		d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
		if err != nil {
			continue
		}
		// Schema is "date\tkind\tlabel[\thours]". Older rows may be
		// "date\tlabel[\thours]" (no kind) — we treat those as holiday.
		var entry DayOff
		entry.Date = d
		entry.Target = 0
		switch len(parts) {
		case 2:
			entry.Kind = KindHoliday
			entry.Label = strings.TrimSpace(parts[1])
		default:
			second := strings.TrimSpace(parts[1])
			if k, ok := ParseKind(second); ok {
				entry.Kind = k
				entry.Label = strings.TrimSpace(parts[2])
				if len(parts) >= 4 {
					if hours, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil && hours >= 0 {
						entry.Target = time.Duration(hours * float64(time.Hour))
					}
				}
			} else {
				// Legacy: second column is label, optional third = hours.
				entry.Kind = KindHoliday
				entry.Label = second
				if len(parts) >= 3 {
					if hours, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err == nil && hours >= 0 {
						entry.Target = time.Duration(hours * float64(time.Hour))
					}
				}
			}
		}
		out[d.Format("2006-01-02")] = entry
	}
	return out
}

// IsDayOff reports whether date is configured as any kind of day off.
func IsDayOff(date time.Time) bool {
	_, ok := loadDayOffs()[date.Format("2006-01-02")]
	return ok
}

// LookupDayOff returns the entry and a found-flag.
func LookupDayOff(date time.Time) (DayOff, bool) {
	d, ok := loadDayOffs()[date.Format("2006-01-02")]
	return d, ok
}

// IsHoliday is back-compat for older call sites — true for any kind of day off.
func IsHoliday(date time.Time) bool { return IsDayOff(date) }

// HolidayName returns the label of a configured day off, or "" if none.
func HolidayName(date time.Time) string {
	if d, ok := LookupDayOff(date); ok {
		return d.Label
	}
	return ""
}

// HolidayTarget returns the target override for a day off, or -1 when the
// date is not configured. 0 means "full day off".
func HolidayTarget(date time.Time) time.Duration {
	if d, ok := LookupDayOff(date); ok {
		return d.Target
	}
	return -1
}

// ListDayOffs returns all entries with from <= date <= to (zero from/to means
// "no bound on that side"), sorted ascending by date.
func ListDayOffs(from, to time.Time) []DayOff {
	all := loadDayOffs()
	out := make([]DayOff, 0, len(all))
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

// AddDayOff appends or replaces a day-off entry. Idempotent for identical
// (date, kind, label) tuples. If a different entry already exists for the
// same date, it is replaced.
func AddDayOff(date time.Time, kind Kind, label string) error {
	if _, ok := ParseKind(string(kind)); !ok {
		return fmt.Errorf("ungültige kategorie: %q", kind)
	}
	label = sanitizeField(label)
	if label == "" {
		label = kind.LabelDe()
	}
	all := readAllDayOffsForWrite()
	all[date.Format("2006-01-02")] = DayOff{
		Date:   time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local),
		Kind:   kind,
		Label:  label,
		Target: 0,
	}
	return writeDayOffs(all)
}

// AddDayOffRange adds an entry for every calendar day in [from, to] (inclusive
// on both ends). Useful for vacation blocks. Returns the count of added rows.
func AddDayOffRange(from, to time.Time, kind Kind, label string) (int, error) {
	if to.Before(from) {
		return 0, errors.New("to liegt vor from")
	}
	cur := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local)
	count := 0
	for !cur.After(end) {
		if err := AddDayOff(cur, kind, label); err != nil {
			return count, err
		}
		count++
		cur = cur.AddDate(0, 0, 1)
	}
	return count, nil
}

// RemoveDayOff deletes the entry for date, if any. Removing a non-existent
// entry is a no-op.
func RemoveDayOff(date time.Time) error {
	all := readAllDayOffsForWrite()
	key := date.Format("2006-01-02")
	if _, ok := all[key]; !ok {
		return nil
	}
	delete(all, key)
	return writeDayOffs(all)
}

// readAllDayOffsForWrite returns a fresh copy from disk (bypassing the
// process-wide cache) so concurrent edits don't lose entries.
func readAllDayOffsForWrite() map[string]DayOff {
	return readDayOffsFromFile(dayoffsPath())
}

// writeDayOffs persists the given map atomically and invalidates the cache.
func writeDayOffs(m map[string]DayOff) error {
	path := dayoffsPath()
	if path == "" {
		return errors.New("kein speicherort verfügbar")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "# worktime day-offs — TSV: date\\tkind\\tlabel[\\thours]"); err != nil {
		f.Close() //nolint:errcheck
		return err
	}
	if _, err := fmt.Fprintln(f, "# kinds: holiday | vacation | sick"); err != nil {
		f.Close() //nolint:errcheck
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
			f.Close() //nolint:errcheck
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	invalidateDayOffCache()
	return nil
}
