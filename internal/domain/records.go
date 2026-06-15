package domain

import (
	"sort"
	"time"
)

// BuildDayRecords groups sessions by the calendar day of their Start in now's
// location and produces one DayRecord per day with sessions present. Grouping
// uses now.Location() (not the session's stored zone) because Postgres returns
// timestamptz as UTC: a session started at 23:30 local would otherwise land in
// the wrong (UTC) day, and Today/Week — which key days in now's location —
// would miss it. Elapsed per session is Stop-Start for finished sessions and
// now-Start for the running one (its live tail thus lands in today's Total).
// Target is filled per day via targetFor. Records are returned chronologically.
func BuildDayRecords(sessions []WorkSession, now time.Time, targetFor func(time.Time) time.Duration) []DayRecord {
	loc := now.Location()
	byDay := map[string]*DayRecord{}
	for _, s := range sessions {
		ls := s.Start.In(loc)
		day := time.Date(ls.Year(), ls.Month(), ls.Day(), 0, 0, 0, 0, loc)
		key := day.Format("2006-01-02")
		el := s.Elapsed(now)
		if el < 0 {
			el = 0
		}
		rec, ok := byDay[key]
		if !ok {
			rec = &DayRecord{Date: day, Target: targetFor(day)}
			byDay[key] = rec
		}
		rec.Total += el
		rec.Sessions = append(rec.Sessions, RecordSession{Tag: s.Tag, Elapsed: el})
	}
	out := make([]DayRecord, 0, len(byDay))
	for _, r := range byDay {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
