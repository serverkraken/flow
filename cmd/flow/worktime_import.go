package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// berlinLoc is the timezone all legacy worktime timestamps are interpreted in.
// Loaded once; falls back to UTC only if the tz database is unavailable.
var berlinLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// parseDateTimeBerlin builds a time from a "2006-01-02" date and a "15:04"
// clock string, interpreted in Europe/Berlin.
func parseDateTimeBerlin(date, hhmm string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, berlinLoc)
}

// logEntry is one parsed worktime.log row. Seconds is the legacy elapsed-seconds
// column; it is captured for a divergence warning but never used for time math.
type logEntry struct {
	Line        int
	Start, Stop time.Time
	Seconds     int
}

type dayOffEntry struct {
	Line      int
	Date      string
	Kind      domain.Kind
	Label     string
	TargetMin int
}

// parseDayOffLine parses "date<TAB>kind<TAB>label[<TAB>hours]". ok=false for
// blank and "#"-comment lines. The Kind may be KindHoliday; the caller skips
// holidays (the server refuses to store them).
func parseDayOffLine(lineNo int, raw string) (dayOffEntry, bool, error) {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasPrefix(t, "#") {
		return dayOffEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) < 3 {
		return dayOffEntry{}, false, fmt.Errorf("expected at least 3 tab-separated columns, got %d", len(f))
	}
	if _, err := time.Parse("2006-01-02", f[0]); err != nil {
		return dayOffEntry{}, false, fmt.Errorf("date: %w", err)
	}
	kind, ok := domain.ParseKind(f[1])
	if !ok {
		return dayOffEntry{}, false, fmt.Errorf("unknown kind %q", f[1])
	}
	e := dayOffEntry{Line: lineNo, Date: f[0], Kind: kind, Label: strings.TrimSpace(f[2])}
	if len(f) >= 4 && strings.TrimSpace(f[3]) != "" {
		hours, err := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		if err != nil {
			return dayOffEntry{}, false, fmt.Errorf("hours: %w", err)
		}
		e.TargetMin = int(hours * 60)
	}
	return e, true, nil
}

// parseLogLine parses a tab-separated "date<TAB>start<TAB>end<TAB>seconds" row.
// ok=false marks a blank line (skip, no error); a non-nil error marks a
// malformed row.
func parseLogLine(lineNo int, raw string) (logEntry, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return logEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) != 4 {
		return logEntry{}, false, fmt.Errorf("expected 4 tab-separated columns, got %d", len(f))
	}
	start, err := parseDateTimeBerlin(f[0], f[1])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("start: %w", err)
	}
	stop, err := parseDateTimeBerlin(f[0], f[2])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("stop: %w", err)
	}
	secs, err := strconv.Atoi(strings.TrimSpace(f[3]))
	if err != nil {
		return logEntry{}, false, fmt.Errorf("seconds: %w", err)
	}
	return logEntry{Line: lineNo, Start: start, Stop: stop, Seconds: secs}, true, nil
}
