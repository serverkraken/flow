package domain

import "time"

// ExpandRange expands an inclusive [from, to] date span into one DayOff per
// day. Dates are normalized to midnight in from's location. If to < from the
// result is nil. When skipWeekends is true, Saturdays and Sundays are
// omitted. Every produced entry carries the same kind, label and targetPerDay
// (0 = full day off, >0 = half-day override).
func ExpandRange(from, to time.Time, kind Kind, label string, targetPerDay time.Duration, skipWeekends bool) []DayOff {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, from.Location())
	if to.Before(from) {
		return nil
	}
	var out []DayOff
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if skipWeekends && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday) {
			continue
		}
		out = append(out, DayOff{Date: day, Kind: kind, Label: label, Target: targetPerDay})
	}
	return out
}
