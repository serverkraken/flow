package wtfmt

import (
	"fmt"
	"strings"
	"time"
)

// ParseHM parses a "HH:MM" string into a time.Duration.
func ParseHM(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("invalid HH:MM %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// NormalizeDurationArg strips leading whitespace and an optional leading "+"
// so that "+1h30m" and "1h30m" both parse via time.ParseDuration.
func NormalizeDurationArg(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "+")
}

// ParseStop resolves a stop-time argument relative to start.
// arg may be a duration string (e.g. "1h30m") or an absolute "HH:MM".
// The now parameter is accepted for API compatibility but is not used.
func ParseStop(arg string, start, _ time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(arg); err == nil {
		return start.Add(d), nil
	}
	hm, err := ParseHM(arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid stop %q", arg)
	}
	base := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return base.Add(hm), nil
}
