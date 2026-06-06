// Package worktime renders the WebUI worktime surface at `/worktime`
// with four sub-tabs (Heute / Woche / Verlauf / Frei). All data
// resolution happens in the handler; templates only render formatted
// strings off a flat view-model.
//
// Shared format helpers (HHMM / signed-HHMM / hour-mask / Monday-of /
// German date headers / weekday-short / month table) live in
// internal/webui/format/. This file keeps worktime-specific aggregators
// (session rows, week-chart bars, saldo sparkline, project/tag shares)
// + the German labels that only the worktime surface uses
// (long weekday, day-label, week-range, relative-day).
package worktime

import (
	"fmt"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/format"
)

// FormatElapsedHumane renders a duration as "2h 14m 06s" / "42m" / "8s".
// Differs from format.FormatHHMM: drops zero-leading hours and exposes
// seconds when the elapsed time is under a minute. Used for the
// live-banner.
func FormatElapsedHumane(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
}

// FormatGermanWeekRange renders "KW 23 · 01. — 07. Juni" — used by the
// /worktime?tab=woche header eyebrow.
func FormatGermanWeekRange(monday time.Time) string {
	sunday := monday.AddDate(0, 0, 6)
	_, week := monday.ISOWeek()
	if monday.Month() == sunday.Month() {
		return fmt.Sprintf("KW %d · %02d. — %02d. %s", week, monday.Day(), sunday.Day(), format.GermanMonth(monday.Month()))
	}
	return fmt.Sprintf("KW %d · %02d. %s — %02d. %s",
		week,
		monday.Day(), format.GermanMonth(monday.Month()),
		sunday.Day(), format.GermanMonth(sunday.Month()),
	)
}

// FormatGermanDayLabel renders "Sa · 06. Juni 2026" for a long history row.
func FormatGermanDayLabel(t time.Time) string {
	return fmt.Sprintf("%s · %02d. %s %d",
		germanWeekdayLong(t.Weekday()),
		t.Day(),
		format.GermanMonth(t.Month()),
		t.Year(),
	)
}

// FormatGermanDayShort renders "Sa · 06.06.2026" — compact form used by
// the Verlauf jump-header for non-relative dates.
func FormatGermanDayShort(t time.Time) string {
	return fmt.Sprintf("%s · %02d.%02d.%d",
		format.GermanWeekdayShort(t.Weekday()),
		t.Day(),
		int(t.Month()),
		t.Year(),
	)
}

// RelativeDayLabel returns a relative-day vocabulary label for `target`
// against `now` (vorgestern / gestern / heute / morgen / übermorgen),
// falling back to a short German date ("Sa · 06.06.2026") for anything
// further out. Used by the Verlauf jump-header to give Soenne a quick
// orientation around the current day.
func RelativeDayLabel(target, now time.Time) string {
	switch {
	case domain.SameDay(target, now.AddDate(0, 0, -2)):
		return "vorgestern"
	case domain.SameDay(target, now.AddDate(0, 0, -1)):
		return "gestern"
	case domain.SameDay(target, now):
		return "heute"
	case domain.SameDay(target, now.AddDate(0, 0, 1)):
		return "morgen"
	case domain.SameDay(target, now.AddDate(0, 0, 2)):
		return "übermorgen"
	default:
		return FormatGermanDayShort(target)
	}
}

func germanWeekdayLong(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "Montag"
	case time.Tuesday:
		return "Dienstag"
	case time.Wednesday:
		return "Mittwoch"
	case time.Thursday:
		return "Donnerstag"
	case time.Friday:
		return "Freitag"
	case time.Saturday:
		return "Samstag"
	default:
		return "Sonntag"
	}
}

// ProjectShareRow is one row in the "Nach Projekt" + "Verteilung" rail
// lists. Pre-formatted; templates only render the strings.
type ProjectShareRow struct {
	Label     string // "flow"
	Total     string // "5:29"
	SharePct  int    // 0..100
	IsRunning bool   // true → fill bar in active color
}

// TagShareRow mirrors ProjectShareRow for tag aggregates.
type TagShareRow struct {
	Label    string
	Total    string
	SharePct int
}

// AggregateProjectShares groups sessions by ProjectID, resolves names via
// `name` (caller-supplied resolver), and returns rows sorted desc by
// total time. Tail rows beyond `topN` are folded into a single
// "übrige" entry — the mockup uses 4 + übrige.
func AggregateProjectShares(
	sessions []domain.Session,
	now time.Time,
	active *domain.ActiveSession,
	name func(projectID string) string,
	topN int,
) ([]ProjectShareRow, time.Duration) {
	totals := make(map[string]time.Duration)
	runningIDs := make(map[string]bool)
	var grand time.Duration
	for _, s := range sessions {
		totals[s.ProjectID] += s.Elapsed
		grand += s.Elapsed
	}
	if active != nil {
		extra := now.Sub(active.StartedAt)
		if extra < 0 {
			extra = 0
		}
		totals[active.ProjectID] += extra
		runningIDs[active.ProjectID] = true
		grand += extra
	}
	type entry struct {
		id    string
		total time.Duration
	}
	entries := make([]entry, 0, len(totals))
	for id, t := range totals {
		entries = append(entries, entry{id: id, total: t})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].total > entries[j].total })

	out := make([]ProjectShareRow, 0, topN+1)
	var rest time.Duration
	for i, e := range entries {
		if i < topN {
			pct := 0
			if grand > 0 {
				pct = int(e.total * 100 / grand)
			}
			out = append(out, ProjectShareRow{
				Label:     name(e.id),
				Total:     format.FormatHHMM(e.total),
				SharePct:  pct,
				IsRunning: runningIDs[e.id],
			})
			continue
		}
		rest += e.total
	}
	if rest > 0 {
		pct := 0
		if grand > 0 {
			pct = int(rest * 100 / grand)
		}
		out = append(out, ProjectShareRow{
			Label:    "übrige",
			Total:    format.FormatHHMM(rest),
			SharePct: pct,
		})
	}
	return out, grand
}

// AggregateTagShares groups sessions by Tag. Sessions without a tag are
// folded into "ohne tag" rather than dropped — the rail otherwise shows a
// surprising sub-total. Sorted desc by total time, no topN folding (the
// tag cardinality is small in practice).
func AggregateTagShares(sessions []domain.Session, now time.Time, active *domain.ActiveSession) []TagShareRow {
	totals := make(map[string]time.Duration)
	var grand time.Duration
	for _, s := range sessions {
		key := s.Tag
		if key == "" {
			key = "ohne tag"
		}
		totals[key] += s.Elapsed
		grand += s.Elapsed
	}
	if active != nil {
		extra := now.Sub(active.StartedAt)
		if extra < 0 {
			extra = 0
		}
		key := active.Tag
		if key == "" {
			key = "ohne tag"
		}
		totals[key] += extra
		grand += extra
	}
	type entry struct {
		tag   string
		total time.Duration
	}
	entries := make([]entry, 0, len(totals))
	for tag, t := range totals {
		entries = append(entries, entry{tag: tag, total: t})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].total > entries[j].total })
	out := make([]TagShareRow, 0, len(entries))
	for _, e := range entries {
		pct := 0
		if grand > 0 {
			pct = int(e.total * 100 / grand)
		}
		out = append(out, TagShareRow{
			Label:    e.tag,
			Total:    format.FormatHHMM(e.total),
			SharePct: pct,
		})
	}
	return out
}

// SessionRow is one row in the "Sessions heute" / "Verlauf" tables.
type SessionRow struct {
	ID          string
	TimeLabel   string // "08:02 — 09:13" or "09:28 — jetzt"
	ProjectName string
	Tag         string
	Note        string
	Duration    string // "1:11" or "2:14"
	IsActive    bool   // → row gets flash-row + active duration color
}

// PauseRow is a divider between two sessions when the gap exceeds the
// pause threshold (default 30 min). The template renders it as a single
// centered eyebrow row inside the same table body so column alignment
// stays sharp.
type PauseRow struct {
	StartLabel string // "12:00"
	EndLabel   string // "13:30"
	Duration   string // "1:30"
}

// TableEntry is one entry in the Sessions table — either a SessionRow
// or a PauseRow divider. Templates switch on IsPause.
type TableEntry struct {
	IsPause bool
	Session SessionRow
	Pause   PauseRow
}

// pauseThreshold is the minimum idle window between two adjacent
// finished sessions that triggers a "— Pause — " divider in the
// sessions table. 30 min mirrors the spec ("gap > 30 min").
const pauseThreshold = 30 * time.Minute

// BuildSessionRows turns finished sessions + the live active session
// into a chronologically-ordered list of table entries, inserting a
// PauseRow divider between any two sessions whose gap exceeds
// pauseThreshold.
//
// Active session, if any, is rendered last with "jetzt" as its end
// label. Project names are resolved via `name`.
func BuildSessionRows(
	sessions []domain.Session,
	active *domain.ActiveSession,
	now time.Time,
	loc *time.Location,
	name func(projectID string) string,
) []TableEntry {
	sorted := make([]domain.Session, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	out := make([]TableEntry, 0, len(sorted)+1)
	var prevStop time.Time
	for i, s := range sorted {
		if i > 0 {
			gap := s.Start.Sub(prevStop)
			if gap > pauseThreshold {
				out = append(out, TableEntry{
					IsPause: true,
					Pause: PauseRow{
						StartLabel: prevStop.In(loc).Format("15:04"),
						EndLabel:   s.Start.In(loc).Format("15:04"),
						Duration:   format.FormatHHMM(gap),
					},
				})
			}
		}
		out = append(out, TableEntry{
			Session: SessionRow{
				ID:          s.ID,
				TimeLabel:   fmt.Sprintf("%s — %s", s.Start.In(loc).Format("15:04"), s.Stop.In(loc).Format("15:04")),
				ProjectName: name(s.ProjectID),
				Tag:         s.Tag,
				Note:        s.Note,
				Duration:    format.FormatHHMM(s.Elapsed),
			},
		})
		prevStop = s.Stop
	}
	if active != nil {
		if !prevStop.IsZero() {
			gap := active.StartedAt.Sub(prevStop)
			if gap > pauseThreshold {
				out = append(out, TableEntry{
					IsPause: true,
					Pause: PauseRow{
						StartLabel: prevStop.In(loc).Format("15:04"),
						EndLabel:   active.StartedAt.In(loc).Format("15:04"),
						Duration:   format.FormatHHMM(gap),
					},
				})
			}
		}
		out = append(out, TableEntry{
			Session: SessionRow{
				TimeLabel:   fmt.Sprintf("%s — jetzt", active.StartedAt.In(loc).Format("15:04")),
				ProjectName: name(active.ProjectID),
				Tag:         active.Tag,
				Note:        active.Note,
				Duration:    format.FormatHHMM(now.Sub(active.StartedAt)),
				IsActive:    true,
			},
		})
	}
	return out
}

// WeekBar is one bar in the week chart (Mo-So). The renderer hands a
// slice of 7 bars to the template; the template emits them as a JSON
// blob for ApexCharts to consume.
type WeekBar struct {
	Label   string  `json:"label"`   // "Mo · 01.06"
	Hours   float64 `json:"hours"`   // 7.5
	HHMM    string  `json:"hhmm"`    // "7:30"
	IsToday bool    `json:"isToday"` // highlights bar + label
}

// BuildWeekBars projects the WeekDay rows from ServerWorktimeView.Week
// into chart-ready bars. Missing days (filtered out by the view because
// they are empty weekend rows) are padded back in so the X axis always
// shows Mo-So.
func BuildWeekBars(week []domain.WeekDay, monday time.Time, now time.Time) []WeekBar {
	byDate := make(map[string]domain.WeekDay, len(week))
	for _, wd := range week {
		byDate[wd.Date.Format("2006-01-02")] = wd
	}
	out := make([]WeekBar, 7)
	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		wd, has := byDate[key]
		var hours time.Duration
		isToday := false
		if has {
			hours = wd.Total(now)
			isToday = wd.IsToday
		} else {
			isToday = domain.SameDay(d, now)
		}
		out[i] = WeekBar{
			Label:   fmt.Sprintf("%s · %02d.%02d", format.GermanWeekdayShort(d.Weekday()), d.Day(), int(d.Month())),
			Hours:   hours.Hours(),
			HHMM:    format.FormatHHMM(hours),
			IsToday: isToday,
		}
	}
	return out
}

// WeekSaldoBar is one bar in the 12-week saldo sparkline. Pos=true →
// active color; Pos=false → err color.
type WeekSaldoBar struct {
	WeekLabel string  `json:"label"` // "KW 23"
	Saldo     float64 `json:"saldo"` // signed hours
	Pos       bool    `json:"pos"`
	IsCurrent bool    `json:"isCurrent"` // highlights the running column
}

// BuildWeekSaldoSeries aggregates sessions in [from, to] into ISO-week
// buckets and emits one bar per week with the saldo = logged − target.
// Target uses defaultTarget for Mon-Fri and 0 for Sat/Sun, mirroring
// ServerWorktimeView.targetFor — see TODO at top of file about pulling
// that into a shared helper.
//
// Target convention for the current week: only days that are FULLY past
// count toward the week's target. Monday morning has weekdayIdx=1, so
// daysDone = 0 → target = 0, saldo = logged − 0. That keeps the bar from
// going negative just because Soenne hasn't started yet today — the bar
// fills in as hours are logged, and "complete day" only locks in after
// midnight rolls over. Past weeks always count all five weekdays
// (Mo-Fr) toward target.
func BuildWeekSaldoSeries(
	sessions []domain.Session,
	from, to time.Time,
	defaultTarget time.Duration,
	now time.Time,
) []WeekSaldoBar {
	// Map of ISO-week-key ("2026-W23") → logged duration.
	logged := make(map[string]time.Duration)
	for _, s := range sessions {
		y, w := s.Date.ISOWeek()
		logged[fmt.Sprintf("%d-W%02d", y, w)] += s.Elapsed
	}
	// Iterate week starts Mo across the [from, to] range.
	cursor := format.MondayOf(from)
	end := format.MondayOf(to)
	out := []WeekSaldoBar{}
	nowY, nowW := now.ISOWeek()
	for !cursor.After(end) {
		y, w := cursor.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", y, w)
		target := time.Duration(0)
		for i := 0; i < 5; i++ { // Mo-Fr only
			target += defaultTarget
		}
		saldo := logged[key] - target
		isCurrent := (y == nowY && w == nowW)
		// For the current (in-progress) week, partial-target adjustment:
		// only count target up to today's weekday + the elapsed share of
		// today doesn't count to keep this sparkline interpretable.
		if isCurrent {
			weekdayIdx := int(now.Weekday())
			if weekdayIdx == 0 {
				weekdayIdx = 7
			}
			daysDone := weekdayIdx - 1 // Mon=0 days complete before Mon, etc.
			if daysDone < 0 {
				daysDone = 0
			}
			if daysDone > 5 {
				daysDone = 5
			}
			target = time.Duration(daysDone) * defaultTarget
			saldo = logged[key] - target
		}
		out = append(out, WeekSaldoBar{
			WeekLabel: fmt.Sprintf("KW %d", w),
			Saldo:     saldo.Hours(),
			Pos:       saldo >= 0,
			IsCurrent: isCurrent,
		})
		cursor = cursor.AddDate(0, 0, 7)
	}
	return out
}

// SumDurations returns the total elapsed across a slice of sessions.
func SumDurations(sessions []domain.Session) time.Duration {
	var total time.Duration
	for _, s := range sessions {
		total += s.Elapsed
	}
	return total
}

// hourTitle renders the tooltip text for a daybar cell. "(jetzt)" suffix
// when the cell is the current hour.
func hourTitle(hour int, isNow bool) string {
	if isNow {
		return fmt.Sprintf("%02d:00 (jetzt)", hour)
	}
	return fmt.Sprintf("%02d:00", hour)
}

// axisLabelFor renders an integer hour as a single-digit-or-two label
// for the daybar axis (0/6/12/18 only get labels).
func axisLabelFor(hour int) string {
	return fmt.Sprintf("%d", hour)
}

// sharePctLabel renders a 0..100 integer as "12%" with a non-breaking
// space inside the cell so the percent sign never wraps.
func sharePctLabel(pct int) string {
	return fmt.Sprintf("%d%%", pct)
}

func daysBookedLabel(n int) string {
	if n == 1 {
		return "1 Tag gebucht"
	}
	return fmt.Sprintf("%d Tage gebucht", n)
}

func weekSaldoSubLabel(pos bool, saldo string) string {
	if pos {
		return saldo + " über Soll"
	}
	return "noch " + trimSign(saldo) + " bis Soll"
}

func trimSign(s string) string {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// barFillStyle returns an inline `style="width: NN%"` value used by the
// horizontal proj-bar fill. Clamps to [0, 100].
func barFillStyle(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("width: %d%%;", pct)
}

func intLabel(n int) string { return fmt.Sprintf("%d", n) }
