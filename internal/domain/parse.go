package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseHM parses an "HH:MM" string into a time.Duration offset from midnight.
//
// Hours must be in [0,24] (24:00 is accepted as the end-of-day idiom),
// minutes in [0,59]. Without this validation, a corrupted log row like
// "99:99" would parse to 100h39m and silently propagate into stats /
// burndown / brief / ICS export, where it would render as "04:39" and
// corrupt accumulators.
func ParseHM(s string) (time.Duration, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid HH:MM: %s", s)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, err
	}
	if h < 0 || h > 24 {
		return 0, fmt.Errorf("invalid HH:MM (hour out of range): %s", s)
	}
	if m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid HH:MM (minute out of range): %s", s)
	}
	if h == 24 && m != 0 {
		return 0, fmt.Errorf("invalid HH:MM (24:MM only valid as 24:00): %s", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// ParseStartArg parses a start-time argument anchored at `now`:
//
//	""        → now
//	"HH:MM"   → that time today (must not be in the future relative to now)
//	"-Nm"     → now - N minutes
//	"-NhMMm"  → now - Nh MMm
func ParseStartArg(arg string, now time.Time) (time.Time, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return now, nil
	}
	if t, ok, err := parseStartArgClock(arg, now); ok {
		return t, err
	}
	if t, ok, err := parseStartArgRelative(arg, now); ok {
		return t, err
	}
	return time.Time{}, fmt.Errorf("unbekanntes Format: %s (erwartet: HH:MM, -1h30m, -45m)", arg)
}

// parseStartArgClock handles the HH:MM shape — that-time-today, future
// rejected. ok=true only when the input matches the clock pattern;
// ParseStartArg falls through to the relative parser otherwise.
// Delegates to ParseHM for trim + range validation, so direct CLI
// callers behave the same as the TSV parser.
func parseStartArgClock(arg string, now time.Time) (time.Time, bool, error) {
	if !strings.Contains(arg, ":") || (len(arg) > 0 && arg[0] == '-') {
		return time.Time{}, false, nil
	}
	hm, err := ParseHM(arg)
	if err != nil {
		return time.Time{}, false, nil
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(hm)
	if t.After(now) {
		return time.Time{}, true, fmt.Errorf("zeit liegt in der Zukunft: %s", arg)
	}
	return t, true, nil
}

// parseStartArgRelative handles `-Nm` and `-NhMMm` shapes. ok=true
// when the input has the leading `-` and parses cleanly; the caller
// surfaces the parse-or-format error.
func parseStartArgRelative(arg string, now time.Time) (time.Time, bool, error) {
	if len(arg) <= 1 || arg[0] != '-' {
		return time.Time{}, false, nil
	}
	rest := arg[1:]
	if hi := strings.IndexByte(rest, 'h'); hi >= 0 {
		hStr := rest[:hi]
		mStr := strings.TrimSuffix(rest[hi+1:], "m")
		h, errH := strconv.Atoi(hStr)
		if errH != nil || h < 0 {
			return time.Time{}, true, fmt.Errorf("ungültig (Stunden-Teil nicht numerisch): %s", arg)
		}
		m := 0
		if mStr != "" {
			var errM error
			m, errM = strconv.Atoi(mStr)
			if errM != nil || m < 0 {
				return time.Time{}, true, fmt.Errorf("ungültig (Minuten-Teil nicht numerisch): %s", arg)
			}
		}
		return now.Add(-time.Duration(h)*time.Hour - time.Duration(m)*time.Minute), true, nil
	}
	// -Nm. Reject negative N (e.g. "--5m") which would otherwise
	// double-negate to "now + 5min" — silently shifting the start
	// into the future on a typo.
	mStr := strings.TrimSuffix(rest, "m")
	min, err := strconv.Atoi(mStr)
	if err != nil || min < 0 {
		return time.Time{}, false, nil
	}
	return now.Add(-time.Duration(min) * time.Minute), true, nil
}

// ParseStop parses a stop-time argument relative to a known start time and
// the current `now`:
//
//   - ""          → now
//   - "HH:MM"     → absolute time today (delegates to ParseStartArg)
//   - "-Nh / -Nm" → now - offset (delegates to ParseStartArg)
//   - "+Nh / +Nm" → start + offset (duration shorthand)
//
// The +Nh form is the reason this is separate from ParseStartArg —
// "stop = start + 90m" is the natural way to enter a session of known
// length, and ParseStartArg has no anchor to add to.
func ParseStop(arg string, start, now time.Time) (time.Time, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return now, nil
	}
	if len(arg) > 1 && arg[0] == '+' {
		dur, err := parseHumanDuration(arg[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("dauer: %w", err)
		}
		if dur <= 0 {
			return time.Time{}, errors.New("dauer muss positiv sein")
		}
		return start.Add(dur), nil
	}
	return ParseStartArg(arg, now)
}

// parseHumanDuration parses "1h30m" / "90m" / "2h" / "45" (minutes) into a
// duration. Internal helper used by ParseStop, which guarantees a non-empty
// trimmed input — no defensive empty-check here.
func parseHumanDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if hi := strings.IndexByte(s, 'h'); hi >= 0 {
		hStr := s[:hi]
		mStr := strings.TrimSuffix(s[hi+1:], "m")
		h, errH := strconv.Atoi(hStr)
		if errH != nil {
			return 0, fmt.Errorf("stunden ungültig: %s", hStr)
		}
		m := 0
		if mStr != "" {
			var errM error
			m, errM = strconv.Atoi(mStr)
			if errM != nil {
				return 0, fmt.Errorf("minuten ungültig: %s", mStr)
			}
		}
		return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
	}
	// "Nm" or bare "N" (minutes)
	mStr := strings.TrimSuffix(s, "m")
	m, err := strconv.Atoi(mStr)
	if err != nil {
		return 0, fmt.Errorf("ungültig: %s", s)
	}
	return time.Duration(m) * time.Minute, nil
}

// ParseRange resolves a friendly range expression anchored at `now`:
//
//   - ""           → all
//   - "today"      → today only
//   - "week"       → ISO Mon..Sun of the current week
//   - "month"      → first..last of current month
//   - "YYYY-MM"    → that month
//   - "YYYY"       → that year
//   - "FROM..TO"   → both YYYY-MM-DD, inclusive
func ParseRange(now time.Time, expr string) (Range, error) {
	switch expr {
	case "":
		return Range{}, nil
	case "today":
		from := startOfDay(now)
		return Range{From: from, To: from.AddDate(0, 0, 1)}, nil
	case "week":
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := startOfDay(now).AddDate(0, 0, -(wd - 1))
		return Range{From: mon, To: mon.AddDate(0, 0, 7)}, nil
	case "month":
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return Range{From: from, To: from.AddDate(0, 1, 0)}, nil
	}
	if len(expr) == 4 {
		if y, err := strconv.Atoi(expr); err == nil {
			from := time.Date(y, time.January, 1, 0, 0, 0, 0, now.Location())
			return Range{From: from, To: from.AddDate(1, 0, 0)}, nil
		}
	}
	if len(expr) == 7 && expr[4] == '-' {
		if t, err := time.ParseInLocation("2006-01", expr, now.Location()); err == nil {
			return Range{From: t, To: t.AddDate(0, 1, 0)}, nil
		}
	}
	if from, to, ok := splitRange(expr); ok {
		f, err := time.ParseInLocation("2006-01-02", from, now.Location())
		if err != nil {
			return Range{}, fmt.Errorf("ungültiges From: %s", from)
		}
		t, err := time.ParseInLocation("2006-01-02", to, now.Location())
		if err != nil {
			return Range{}, fmt.Errorf("ungültiges To: %s", to)
		}
		return Range{From: f, To: t.AddDate(0, 0, 1)}, nil
	}
	return Range{}, fmt.Errorf("unbekannter Range: %s", expr)
}

// splitRange splits "FROM..TO" at the first "..". Returns ("", "", false)
// when no separator is found.
func splitRange(s string) (string, string, bool) {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return s[:i], s[i+2:], true
		}
	}
	return "", "", false
}

// ParseDateOrRange parses a single YYYY-MM-DD date or a YYYY-MM-DD..YYYY-MM-DD
// range expression anchored in loc. Returned bounds are inclusive (from == to
// when isRange is false). Used by both the worktime CLI verbs and the
// dayoffs add-range form so the syntax stays uniform. Pass time.Local for
// production behavior; tests pass an explicit location to stay TZ-independent.
// Additionally rejects inverted ranges (a > b).
func ParseDateOrRange(s string, loc *time.Location) (from, to time.Time, isRange bool, err error) {
	if loc == nil {
		loc = time.UTC
	}
	if a, b, ok := splitRange(s); ok {
		from, e1 := time.ParseInLocation("2006-01-02", a, loc)
		to, e2 := time.ParseInLocation("2006-01-02", b, loc)
		if e1 != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("from: %w", e1)
		}
		if e2 != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("to: %w", e2)
		}
		if to.Before(from) {
			return time.Time{}, time.Time{}, false, fmt.Errorf("ungültiger range: %s liegt vor %s", b, a)
		}
		return from, to, true, nil
	}
	d, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("ungültiges datum: %s (YYYY-MM-DD oder YYYY-MM-DD..YYYY-MM-DD)", s)
	}
	return d, d, false, nil
}

// startOfDay returns t truncated to 00:00 in t's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
