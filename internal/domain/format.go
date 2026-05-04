package domain

import (
	"fmt"
	"strings"
	"time"
)

// FmtDuration formats a duration as "Nh MMm" (e.g. "2h 15m"). Negative
// durations are clamped to zero — the format is for display, not math.
//
// Rounding is half-up to the nearest minute so a session ending at
// 1h59m45s renders as "2h 00m" rather than truncating to "1h 59m".
// Without this, the threshold check (>= target) could be satisfied
// while the formatted brief showed "8h 00m / 8h 00m" but no ✓ tick.
// Threshold comparisons still use the raw duration; only the display
// rounds.
func FmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Minute)
	mins := int(d / time.Minute)
	return fmt.Sprintf("%dh %02dm", mins/60, mins%60)
}

// FmtSignedDuration formats a duration with an explicit "+" or "-" sign.
// Zero renders as "+0h 00m" — the sign always appears, signalling that the
// value is a balance/saldo rather than a quantity. Rounding policy
// matches FmtDuration (half-up to the minute, after sign extraction).
func FmtSignedDuration(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	d = d.Round(time.Minute)
	mins := int(d / time.Minute)
	return fmt.Sprintf("%s%dh %02dm", sign, mins/60, mins%60)
}

var (
	weekdayShortDeMap = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	monthShortDeMap   = [13]string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
)

// WeekdayShortDe returns the German two-letter weekday abbreviation.
func WeekdayShortDe(wd time.Weekday) string { return weekdayShortDeMap[wd] }

// MonthShortDe returns the German three-letter month abbreviation. Index 0
// returns "" — time.Month values are 1-based, so this is reachable only
// from buggy callers, but keeping the slot avoids an off-by-one in lookups.
func MonthShortDe(m time.Month) string { return monthShortDeMap[m] }

// IcalEscape escapes the four characters RFC 5545 §3.3.11 requires escaped
// in TEXT-typed values: backslash, semicolon, comma, newline. Carriage
// returns are dropped — the ICS line ending is CRLF and \r in content
// would corrupt it.
func IcalEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\n", `\n`,
		"\r", "",
	)
	return r.Replace(s)
}

// DailyNoteID returns the canonical Kompendium ID for the daily note of a
// given date (e.g. "daily/2026-04-30").
func DailyNoteID(date time.Time) string {
	return "daily/" + date.Format("2006-01-02")
}

// HumanizeNoteID returns a short, human-friendly label derived from a note
// ID alone — useful when the full note metadata isn't loaded (e.g. when
// rendering attached IDs in the today view).
func HumanizeNoteID(id string) string {
	switch {
	case strings.HasPrefix(id, "daily/"):
		return "Daily " + strings.TrimPrefix(id, "daily/")
	case strings.HasPrefix(id, "projects/"):
		return "Projekt " + strings.TrimPrefix(id, "projects/")
	case strings.HasPrefix(id, "notes/"):
		return "Notiz " + strings.TrimPrefix(id, "notes/")
	}
	return id
}
