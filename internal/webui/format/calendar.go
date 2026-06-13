package format

import (
	"fmt"
	"time"
)

// MondayOf returns 00:00 of t's ISO Monday in t's location.
// Duplicated here (with internal/usecase) to avoid pulling that package
// into the template helpers.
func MondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return day.AddDate(0, 0, -(wd - 1))
}

// GermanDateHeader renders "Sa · 06. Juni · KW 23" for the tiny
// header above the two-up numerics. Plain Go date formatting with a
// hand-rolled German weekday + month table — i18n stays out of M6.
func GermanDateHeader(t time.Time) string {
	wd := GermanWeekdayShort(t.Weekday())
	month := GermanMonth(t.Month())
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s · %02d. %s · KW %d", wd, t.Day(), month, week)
}

// GermanWeekdayShort renders a weekday as a two-letter German abbreviation
// (Mo / Di / Mi / Do / Fr / Sa / So).
func GermanWeekdayShort(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "Mo"
	case time.Tuesday:
		return "Di"
	case time.Wednesday:
		return "Mi"
	case time.Thursday:
		return "Do"
	case time.Friday:
		return "Fr"
	case time.Saturday:
		return "Sa"
	default:
		return "So"
	}
}

// GermanMonth renders a month as its full German name ("Januar" …
// "Dezember"). Shared by every date-header / day-label renderer.
func GermanMonth(m time.Month) string {
	switch m {
	case time.January:
		return "Januar"
	case time.February:
		return "Februar"
	case time.March:
		return "März"
	case time.April:
		return "April"
	case time.May:
		return "Mai"
	case time.June:
		return "Juni"
	case time.July:
		return "Juli"
	case time.August:
		return "August"
	case time.September:
		return "September"
	case time.October:
		return "Oktober"
	case time.November:
		return "November"
	default:
		return "Dezember"
	}
}
