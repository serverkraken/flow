// Package worktime implements the worktime data layer: reading/writing
// ~/.tmux/worktime.state and ~/.tmux/worktime.log, plus week/history aggregation.
package worktime

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrNoActiveSession signals an attempt to stop or correct when nothing is running.
// Callers can branch on errors.Is(err, ErrNoActiveSession) to turn it into a no-op.
var ErrNoActiveSession = errors.New("keine aktive Session")

// ErrAlreadyRunning is returned by Start when a session is already active.
// Prevents silent overwrite of the running state — the caller must Stop first
// or call StartForce explicitly.
var ErrAlreadyRunning = errors.New("session läuft bereits")

// ErrOverlap is returned by AddManual / EditSession when the requested span
// overlaps an existing session on the same date. Callers (TUI/CLI) can detect
// this with errors.Is and present a precise hint instead of a generic failure.
var ErrOverlap = errors.New("überschneidet eine bestehende Session")

// TargetHours is the default daily target, configurable via WORKTIME_TARGET_HOURS.
// Per-weekday overrides are loaded from ~/.tmux/worktime.conf via TargetFor.
var TargetHours = func() time.Duration {
	if v := os.Getenv("WORKTIME_TARGET_HOURS"); v != "" {
		if h, err := strconv.ParseFloat(v, 64); err == nil && h > 0 {
			return time.Duration(h * float64(time.Hour))
		}
	}
	return 8 * time.Hour
}()

// TargetFor returns the daily work target for the given date, honouring
// (in order): configured day-offs (Feiertag/Urlaub/Krank) → per-weekday
// overrides from ~/.tmux/worktime.conf → process default TargetHours.
//
// Config format (key=value, # comments, blank lines OK):
//
//	target_hours = 8
//	target_mon = 8
//	target_tue = 8
//	target_wed = 8
//	target_thu = 8
//	target_fri = 6
//	target_sat = 0
//	target_sun = 0
//
// Day-offs always win when present: a holiday on a Wednesday returns 0 (or
// the explicit half-day override) regardless of target_wed.
func TargetFor(date time.Time) time.Duration {
	if t := HolidayTarget(date); t >= 0 {
		return t
	}
	cfg := loadConfig()
	if d, ok := cfg.perDay[date.Weekday()]; ok {
		return d
	}
	if cfg.def > 0 {
		return cfg.def
	}
	return TargetHours
}

type config struct {
	def        time.Duration
	perDay     map[time.Weekday]time.Duration
	tagTargets map[string]time.Duration // per-tag daily target, e.g. tag_target_deep = 4
}

var weekdayKeys = map[string]time.Weekday{
	"target_mon": time.Monday,
	"target_tue": time.Tuesday,
	"target_wed": time.Wednesday,
	"target_thu": time.Thursday,
	"target_fri": time.Friday,
	"target_sat": time.Saturday,
	"target_sun": time.Sunday,
}

// loadConfig reads ~/.tmux/worktime.conf. Any read/parse failure is silent
// and yields an empty config (callers fall back to TargetHours).
//
// Recognised keys:
//
//	target_hours      = 8       # default daily target
//	target_mon..sun   = 8       # per-weekday override
//	tag_target_NAME   = 4       # daily target hours for sessions tagged NAME
func loadConfig() config {
	cfg := config{
		perDay:     map[time.Weekday]time.Duration{},
		tagTargets: map[string]time.Duration{},
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	f, err := os.Open(filepath.Join(home, ".tmux", "worktime.conf"))
	if err != nil {
		return cfg
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		h, err := strconv.ParseFloat(val, 64)
		if err != nil || h < 0 {
			continue
		}
		dur := time.Duration(h * float64(time.Hour))
		if key == "target_hours" {
			cfg.def = dur
			continue
		}
		if wd, ok := weekdayKeys[key]; ok {
			cfg.perDay[wd] = dur
			continue
		}
		if strings.HasPrefix(key, "tag_target_") {
			tag := strings.TrimPrefix(key, "tag_target_")
			if tag != "" {
				cfg.tagTargets[tag] = dur
			}
		}
	}
	return cfg
}

// TagTargets returns the configured per-tag daily targets, keyed by tag name.
// Empty when no tag_target_* keys are present in worktime.conf.
func TagTargets() map[string]time.Duration {
	cfg := loadConfig()
	out := make(map[string]time.Duration, len(cfg.tagTargets))
	for k, v := range cfg.tagTargets {
		out[k] = v
	}
	return out
}

// TagTarget returns the configured daily target for the named tag, or 0 when
// none is set. Lookup is case-insensitive — "deep" and "Deep" hit the same key.
func TagTarget(tag string) time.Duration {
	cfg := loadConfig()
	if v, ok := cfg.tagTargets[tag]; ok {
		return v
	}
	for k, v := range cfg.tagTargets {
		if strings.EqualFold(k, tag) {
			return v
		}
	}
	return 0
}

// Session is a completed work session.
type Session struct {
	Date    time.Time
	Start   time.Time
	Stop    time.Time
	Elapsed time.Duration
	Tag     string // optional category, e.g. "deep", "meeting"
	Note    string // optional one-line annotation
}

// Day holds all sessions for today plus an optional active session.
type Day struct {
	Sessions   []Session
	Active     *time.Time
	PausedAt   *time.Time // stop time of last pause, when in pause-mode (no active session, but pause marker exists)
	Logged     time.Duration
	Target     time.Duration
}

// IsPaused reports whether the user paused (Pause()) and hasn't resumed yet.
// Distinct from "fresh idle" — UI shows "in Pause seit HH:MM" instead of
// "noch nicht erfasst", and Resume() picks up where Pause() left off.
func (d Day) IsPaused() bool { return d.Active == nil && d.PausedAt != nil }

// Total returns logged + active elapsed (capped at midnight for the current day).
func (d Day) Total(now time.Time) time.Duration {
	total := d.Logged
	if d.Active != nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start := *d.Active
		if start.Before(midnight) {
			start = midnight
		}
		total += now.Sub(start)
	}
	return total
}

// IsRunning reports whether a session is currently active.
func (d Day) IsRunning() bool { return d.Active != nil }

// WeekDay is one day in the week view.
type WeekDay struct {
	Date    time.Time
	Logged  time.Duration
	Active  *time.Time
	Target  time.Duration
	IsToday bool
}

// Total returns logged + active elapsed for this day.
func (w WeekDay) Total(now time.Time) time.Duration {
	if !w.IsToday || w.Active == nil {
		return w.Logged
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := *w.Active
	if start.Before(midnight) {
		start = midnight
	}
	return w.Logged + now.Sub(start)
}

// DayRecord is used by the history view.
type DayRecord struct {
	Date     time.Time
	Sessions []Session
	Total    time.Duration
	Target   time.Duration
}

// LoadToday reads today's worktime data.
func LoadToday() (Day, error) {
	now := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		return Day{Target: TargetFor(now)}, err
	}
	base := filepath.Join(home, ".tmux")
	day := Day{Target: TargetFor(now)}

	day.Active, _ = readActiveState(filepath.Join(base, "worktime.state"))
	day.PausedAt, _ = readActiveState(filepath.Join(base, "worktime.pause"))
	if day.Active != nil {
		// Active session always takes precedence over a stale pause marker.
		day.PausedAt = nil
	}

	sessions, err := readSessions(filepath.Join(base, "worktime.log"), func(s Session) bool {
		return sameDay(s.Date, now)
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return day, err
	}
	day.Sessions = sessions
	for _, s := range sessions {
		day.Logged += s.Elapsed
	}
	return day, nil
}

// LoadWeek returns Mon–Sun of the current week (Sat/Sun only if they have sessions).
func LoadWeek(now time.Time) ([]WeekDay, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(home, ".tmux", "worktime.log")
	statePath := filepath.Join(home, ".tmux", "worktime.state")

	active, _ := readActiveState(statePath)

	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := now.AddDate(0, 0, -(wd - 1))
	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location())
	sunday := monday.AddDate(0, 0, 6)

	allSessions, _ := readSessions(logPath, func(s Session) bool {
		return !s.Date.Before(monday) && !s.Date.After(sunday)
	})

	byDay := make(map[string]time.Duration)
	for _, s := range allSessions {
		key := s.Date.Format("2006-01-02")
		byDay[key] += s.Elapsed
	}

	showWeekend := os.Getenv("WORKTIME_SHOW_WEEKEND") == "1"

	var week []WeekDay
	for i := 0; i < 7; i++ {
		day := monday.AddDate(0, 0, i)
		isToday := sameDay(day, now)
		key := day.Format("2006-01-02")
		logged := byDay[key]

		isWeekend := i >= 5
		if isWeekend && logged == 0 && !isToday && !showWeekend {
			continue
		}

		var dayActive *time.Time
		if isToday {
			dayActive = active
		}

		week = append(week, WeekDay{
			Date:    day,
			Logged:  logged,
			Active:  dayActive,
			Target:  TargetFor(day),
			IsToday: isToday,
		})
	}
	return week, nil
}

// LoadHistory returns all days with sessions, newest first.
func LoadHistory() ([]DayRecord, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sessions, err := readSessions(filepath.Join(home, ".tmux", "worktime.log"), nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	byDate := make(map[string]*DayRecord)
	var order []string
	for _, s := range sessions {
		key := s.Date.Format("2006-01-02")
		if _, ok := byDate[key]; !ok {
			byDate[key] = &DayRecord{Date: s.Date, Target: TargetFor(s.Date)}
			order = append(order, key)
		}
		rec := byDate[key]
		rec.Sessions = append(rec.Sessions, s)
		rec.Total += s.Elapsed
	}

	sort.Sort(sort.Reverse(sort.StringSlice(order)))

	result := make([]DayRecord, len(order))
	for i, key := range order {
		result[i] = *byDate[key]
	}
	return result, nil
}

// Start writes the session start time to the state file. Returns
// ErrAlreadyRunning when a session is already active — callers (TUI, CLI)
// must surface that explicitly so the user doesn't silently lose the original
// start time. Use StartForce to overwrite anyway.
func Start(ts time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	statePath := filepath.Join(home, ".tmux", "worktime.state")
	return withLock(home, func() error {
		if active, _ := readActiveState(statePath); active != nil {
			return ErrAlreadyRunning
		}
		return os.WriteFile(statePath, []byte(strconv.FormatInt(ts.Unix(), 10)), 0o644)
	})
}

// StartForce overwrites the state file unconditionally. Used when the user
// explicitly confirms ("trotzdem starten") after seeing an ErrAlreadyRunning.
func StartForce(ts time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return withLock(home, func() error {
		if err := os.WriteFile(
			filepath.Join(home, ".tmux", "worktime.state"),
			[]byte(strconv.FormatInt(ts.Unix(), 10)),
			0o644,
		); err != nil {
			return err
		}
		// Resume clears the pause marker; the next Stop will log a contiguous
		// session, and the user no longer sees "in Pause seit …".
		_ = os.Remove(filepath.Join(home, ".tmux", "worktime.pause"))
		return nil
	})
}

// Pause stops the current session and writes a pause marker. Distinct from
// Stop (no marker) so the UI can render "in Pause seit HH:MM" and the next
// Start clears the marker automatically. No-op (no error) when nothing is
// running — pressing pause twice is harmless.
func Pause() (Session, error) {
	s, err := Stop()
	if err != nil {
		if errors.Is(err, ErrNoActiveSession) {
			return Session{}, nil
		}
		return Session{}, err
	}
	home, hErr := os.UserHomeDir()
	if hErr == nil {
		_ = os.WriteFile(
			filepath.Join(home, ".tmux", "worktime.pause"),
			[]byte(strconv.FormatInt(s.Stop.Unix(), 10)),
			0o644,
		)
	}
	return s, nil
}

// Resume starts a session and clears the pause marker. Equivalent to
// Start(now) followed by clearing the marker — exposed as a separate verb so
// the CLI/TUI can offer "Resume" as a distinct action.
func Resume() error {
	if err := Start(time.Now()); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err == nil {
		_ = os.Remove(filepath.Join(home, ".tmux", "worktime.pause"))
	}
	return nil
}

// Stop stops the current session and logs it. Returns the completed session
// (the *original* span; if it crossed midnight it is also logged as multiple
// per-day rows so each calendar day reflects its own elapsed time).
func Stop() (Session, error) {
	return stopWith(time.Now())
}

func stopWith(stop time.Time) (Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Session{}, err
	}
	statePath := filepath.Join(home, ".tmux", "worktime.state")
	logPath := filepath.Join(home, ".tmux", "worktime.log")

	var s Session
	err = withLock(home, func() error {
		active, err := readActiveState(statePath)
		if err != nil {
			return err
		}
		if active == nil {
			return ErrNoActiveSession
		}
		if !stop.After(*active) {
			return errors.New("stoppzeit muss nach Startzeit liegen")
		}
		s = Session{
			Date:    time.Date(stop.Year(), stop.Month(), stop.Day(), 0, 0, 0, 0, stop.Location()),
			Start:   *active,
			Stop:    stop,
			Elapsed: stop.Sub(*active),
		}
		for _, part := range splitAtMidnight(*active, stop) {
			if err := appendLog(logPath, part); err != nil {
				return err
			}
		}
		if err := os.Remove(statePath); err != nil {
			return err
		}
		// Clear stale pause marker (Stop ends the session for real; Pause
		// re-writes the marker after this returns).
		_ = os.Remove(filepath.Join(home, ".tmux", "worktime.pause"))
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s, nil
}

// Toggle starts the session if idle, stops it if running.
// Returns a human-readable description of the action taken.
func Toggle() (string, error) {
	day, err := LoadToday()
	if err != nil {
		return "", err
	}
	if day.IsRunning() {
		s, err := Stop()
		if err != nil {
			return "", err
		}
		h := int(s.Elapsed.Hours())
		m := int(s.Elapsed.Minutes()) % 60
		return fmt.Sprintf("gestoppt nach %dh %02dm", h, m), nil
	}
	if err := Start(time.Now()); err != nil {
		return "", err
	}
	return "gestartet", nil
}

// CorrectStart overwrites the start time of the currently running session.
func CorrectStart(ts time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	statePath := filepath.Join(home, ".tmux", "worktime.state")
	return withLock(home, func() error {
		active, err := readActiveState(statePath)
		if err != nil || active == nil {
			return ErrNoActiveSession
		}
		return os.WriteFile(statePath, []byte(strconv.FormatInt(ts.Unix(), 10)), 0o644)
	})
}

// StopAt stops the current session with a specific stop time.
func StopAt(stopTime time.Time) (Session, error) {
	return stopWith(stopTime)
}

// DeleteSession removes the session at index idx (0-based) for the given date.
func DeleteSession(date time.Time, idx int) error {
	return rewriteLog(func(sessions []Session) []Session {
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		result := make([]Session, 0, len(sessions))
		for _, s := range sessions {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx != idx {
					result = append(result, s)
				}
				dayIdx++
			} else {
				result = append(result, s)
			}
		}
		return result
	})
}

// EditSession replaces the session at index idx (0-based) for the given date,
// preserving its Tag and Note. Returns ErrOverlap if the new span intersects
// another session on the same day.
func EditSession(date time.Time, idx int, newStart, newStop time.Time) error {
	if !newStop.After(newStart) {
		return errors.New("stoppzeit muss nach Startzeit liegen")
	}
	if hit, conflict, err := SessionsOverlap(date, newStart, newStop, idx); err != nil {
		return err
	} else if hit && conflict != nil {
		return fmt.Errorf("%w (%s → %s)",
			ErrOverlap, conflict.Start.Format("15:04"), conflict.Stop.Format("15:04"))
	}
	return rewriteLog(func(sessions []Session) []Session {
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		for i, s := range sessions {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx == idx {
					sessions[i] = Session{
						Date:    s.Date,
						Start:   newStart,
						Stop:    newStop,
						Elapsed: newStop.Sub(newStart),
						Tag:     s.Tag,
						Note:    s.Note,
					}
				}
				dayIdx++
			}
		}
		return sessions
	})
}

// SetTag sets (or clears, if tag == "") the Tag of the session at index idx
// (0-based) for the given date.
func SetTag(date time.Time, idx int, tag string) error {
	tag = sanitizeField(tag)
	return rewriteLog(func(sessions []Session) []Session {
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		for i, s := range sessions {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx == idx {
					sessions[i].Tag = tag
				}
				dayIdx++
			}
		}
		return sessions
	})
}

// SetNote sets (or clears, if note == "") the Note of the session at index
// idx (0-based) for the given date.
func SetNote(date time.Time, idx int, note string) error {
	note = sanitizeField(note)
	return rewriteLog(func(sessions []Session) []Session {
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		for i, s := range sessions {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx == idx {
					sessions[i].Note = note
				}
				dayIdx++
			}
		}
		return sessions
	})
}

// TopUsageTags returns up to n distinct tags ordered by total session count,
// descending. Ties are broken by most recent use. Used by the TUI tag-form
// to show a "top by usage" suggestion strip alongside the recency strip.
func TopUsageTags(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	sessions, err := loadAllSessions()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	type entry struct {
		tag    string
		count  int
		latest time.Time
	}
	bucket := make(map[string]*entry, len(sessions))
	for _, s := range sessions {
		if s.Tag == "" {
			continue
		}
		e, ok := bucket[s.Tag]
		if !ok {
			e = &entry{tag: s.Tag}
			bucket[s.Tag] = e
		}
		e.count++
		if s.Start.After(e.latest) {
			e.latest = s.Start
		}
	}
	all := make([]*entry, 0, len(bucket))
	for _, e := range bucket {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count > all[j].count
		}
		return all[i].latest.After(all[j].latest)
	})
	out := make([]string, 0, n)
	for _, e := range all {
		out = append(out, e.tag)
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// RecentTags returns up to n distinct tags most recently used, newest first.
// "Recency" is defined by Session.Start. Empty tags are skipped. Used by the
// TUI tag-form for autocomplete.
func RecentTags(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	sessions, err := loadAllSessions()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Start.After(sessions[j].Start) })
	seen := make(map[string]bool, n)
	out := make([]string, 0, n)
	for _, s := range sessions {
		if s.Tag == "" || seen[s.Tag] {
			continue
		}
		seen[s.Tag] = true
		out = append(out, s.Tag)
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// SessionsOverlap reports whether candidate [start, stop) intersects any
// session in the same date in the on-disk log, except the one at excludeIdx
// (use -1 to consider all). Used by AddManual / EditSession to prevent
// double-booking.
func SessionsOverlap(date, start, stop time.Time, excludeIdx int) (bool, *Session, error) {
	all, err := loadAllSessions()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil, nil
		}
		return false, nil, err
	}
	dateStr := date.Format("2006-01-02")
	dayIdx := 0
	for i := range all {
		s := all[i]
		if s.Date.Format("2006-01-02") != dateStr {
			continue
		}
		if dayIdx != excludeIdx {
			if start.Before(s.Stop) && s.Start.Before(stop) {
				return true, &all[i], nil
			}
		}
		dayIdx++
	}
	return false, nil, nil
}

// sanitizeField strips characters that would break the TSV format.
func sanitizeField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// rewriteLog reads the entire log, applies fn, and writes it back atomically.
func rewriteLog(fn func([]Session) []Session) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, ".tmux", "worktime.log")

	return withLock(home, func() error {
		sessions, err := readSessions(logPath, nil)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		sessions = fn(sessions)

		tmp := logPath + ".tmp"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		for _, s := range sessions {
			if _, err := writeSessionLine(f, s); err != nil {
				f.Close() //nolint:errcheck
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, logPath)
	})
}

// AddManual appends a manual session entry to the log.
// If start..stop crosses midnight it is split into one row per day.
// Returns ErrOverlap if the span intersects an existing session on the day(s)
// it would land in.
//
// The first arg (date) is retained for API symmetry with the other write
// operations; the actual stored row's date is derived from start via
// splitAtMidnight. Renamed to _ for the linter — callers may still pass the
// hint by position.
func AddManual(_, start, stop time.Time) error {
	if !stop.After(start) {
		return errors.New("stop muss nach Start liegen")
	}
	for _, part := range splitAtMidnight(start, stop) {
		hit, conflict, err := SessionsOverlap(part.Date, part.Start, part.Stop, -1)
		if err != nil {
			return err
		}
		if hit && conflict != nil {
			return fmt.Errorf("%w (%s, %s → %s)",
				ErrOverlap,
				part.Date.Format("2006-01-02"),
				conflict.Start.Format("15:04"),
				conflict.Stop.Format("15:04"))
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, ".tmux", "worktime.log")
	return withLock(home, func() error {
		for _, part := range splitAtMidnight(start, stop) {
			if err := appendLog(logPath, part); err != nil {
				return err
			}
		}
		return nil
	})
}

// MaxStreakMinutes is the threshold (in minutes) above which the active-session
// indicator switches to a warning color, configurable via WORKTIME_MAX_STREAK_MIN.
// Default 90 (yellow at 90+, red at 120+).
var MaxStreakMinutes = func() int {
	if v := os.Getenv("WORKTIME_MAX_STREAK_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 90
}()

// StatusSegment returns the tmux status-right segment string.
// Returns "" when nothing was tracked today and no week activity exists.
//
// Layout: [Frei: …] ⏱ HH:MM ▶ S:MM ✓ ●●●●○ Streak N
//   - "[Frei: Label]" — when today is a configured day-off (Feiertag/Urlaub/Krank)
//   - "⏱ HH:MM"  — today's total
//   - "▶ S:MM"   — active session length, yellow at >= MaxStreakMinutes, red at >= 2× of it
//   - "→HH:MM"   — projected target-hit time when below target
//   - "✓"        — target reached today
//   - dots       — Mon–Fri pace: green ● hit, yellow ● today running but below, dim ○ missed
//   - "Streak N" — current workday streak when ≥ 3
//
// Same shape (●) is used for both "hit" and "today running" because some fonts
// render half-fill glyphs (◐) at emoji width, which breaks the row.
func StatusSegment() string {
	now := time.Now()
	day, dayErr := LoadToday()
	if dayErr != nil {
		return ""
	}
	total := day.Total(now)

	week, _ := LoadWeek(now) // tolerable on error: dots just stay empty
	dots := buildPaceDots(week, now)

	dayOff, isDayOff := LookupDayOff(now)
	if total == 0 && dots == "" && !isDayOff {
		return ""
	}

	pal := loadStatusPalette()

	h := int(total.Hours())
	m := int(total.Minutes()) % 60
	target := TargetFor(now)

	achieved := target == 0 || total >= target

	var icon, mainColor string
	bold := false
	if day.IsRunning() {
		icon = "⏱"
		bold = true
		switch {
		// Red is reserved for "really a lot" — prevents normal +1h overtime
		// from looking like an alarm.
		case total >= target+4*time.Hour:
			mainColor = pal.red
		case achieved:
			mainColor = pal.green
		case total >= target-2*time.Hour:
			mainColor = pal.yellow
		default:
			mainColor = pal.cyan
		}
	} else {
		icon = "⏸"
		bold = false
		if achieved && total > 0 {
			mainColor = pal.green
		} else {
			mainColor = pal.dim
		}
	}

	mainAttr := mainColor
	if bold {
		mainAttr += ",bold"
	}

	var parts []string
	if isDayOff {
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s %s#[default]",
			pal.cyan, dayOffGlyph(dayOff.Kind), dayOff.Label))
	}
	parts = append(parts, fmt.Sprintf("#[fg=%s]%s %02d:%02d#[default]", mainAttr, icon, h, m))

	if day.IsRunning() && day.Active != nil {
		elapsed := now.Sub(*day.Active)
		if elapsed < 0 {
			elapsed = 0
		}
		eh := int(elapsed.Hours())
		em := int(elapsed.Minutes()) % 60

		streakColor := pal.dim
		minutes := int(elapsed.Minutes())
		// `!` glyph signals "you've been at it a while" in peripheral vision.
		// Color shift alone is easy to miss when the eye scans past; the
		// extra char makes the warning unambiguous.
		glyph := "▶"
		switch {
		case MaxStreakMinutes > 0 && minutes >= 2*MaxStreakMinutes:
			streakColor = pal.red
			glyph = "▶!"
		case MaxStreakMinutes > 0 && minutes >= MaxStreakMinutes:
			streakColor = pal.yellow
			glyph = "▶!"
		}
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s %d:%02d#[default]", streakColor, glyph, eh, em))

		// ETA: when running and below target, show projected hit time —
		// nudges toward "stop now / keep going" without taking up much space.
		if !achieved {
			etaT := day.Active.Add(target - day.Logged)
			parts = append(parts, fmt.Sprintf("#[fg=%s]→%s#[default]",
				pal.dim, etaT.Format("15:04")))
		}
	}

	if achieved && total > 0 {
		parts = append(parts, fmt.Sprintf("#[fg=%s,bold]✓#[default]", pal.green))
	}

	if dots != "" {
		parts = append(parts, dots)
	}

	if streak := CurrentStreak(); streak >= 3 {
		parts = append(parts, fmt.Sprintf("#[fg=%s]Streak %d#[default]", pal.green, streak))
	}

	return strings.Join(parts, " ")
}

// dayOffGlyph picks a monospace TUI marker for each day-off kind. Avoids
// emoji pictograms so the status row's column width stays predictable.
func dayOffGlyph(k Kind) string {
	switch k {
	case KindHoliday:
		return "★"
	case KindVacation:
		return "☼"
	case KindSick:
		return "✚"
	}
	return "·"
}

type statusPalette struct {
	green, yellow, red, cyan, dim string
}

func loadStatusPalette() statusPalette {
	pick := func(opt, fallback string) string {
		if v := tmuxOpt(opt); v != "" {
			return v
		}
		return fallback
	}
	return statusPalette{
		green:  pick("tn_green", "#9ece6a"),
		yellow: pick("tn_yellow", "#e0af68"),
		red:    pick("tn_red", "#f7768e"),
		cyan:   pick("tn_cyan", "#7dcfff"),
		dim:    pick("tn_dim", "#565f89"),
	}
}

// buildPaceDots renders Mon–Fri pace dots for the tmux status segment.
// Returns "" when no weekday in the week has any activity (avoids a stray segment
// at the start of an empty week).
//
// Day-offs (Feiertag/Urlaub/Krank) get a distinct glyph + cyan tint so the
// row tells "I had a free day" instead of "I missed a target".
func buildPaceDots(week []WeekDay, now time.Time) string {
	pal := loadStatusPalette()
	type dot struct {
		ch    string
		color string
	}
	var dots []dot
	any := false
	for _, d := range week {
		if d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday {
			continue
		}
		if dayOff, isOff := LookupDayOff(d.Date); isOff {
			dots = append(dots, dot{statusDayOffGlyph(dayOff.Kind), pal.cyan})
			any = true
			continue
		}
		total := d.Total(now)
		hit := total >= d.Target
		switch {
		case hit:
			dots = append(dots, dot{"●", pal.green})
			any = true
		case d.IsToday && d.Active != nil:
			// Same glyph as "hit", colour-differentiated. Half-fill glyphs
			// like ◐ render at emoji width in some fonts and break alignment.
			dots = append(dots, dot{"●", pal.yellow})
			any = true
		default:
			dots = append(dots, dot{"○", pal.dim})
		}
	}
	if !any {
		return ""
	}
	parts := make([]string, len(dots))
	for i, d := range dots {
		parts[i] = fmt.Sprintf("#[fg=%s]%s#[default]", d.color, d.ch)
	}
	return strings.Join(parts, "")
}

func statusDayOffGlyph(k Kind) string {
	switch k {
	case KindHoliday:
		return "★"
	case KindVacation:
		return "☼"
	case KindSick:
		return "✚"
	}
	return "○"
}

// ParseStartArg parses a start-time argument: "" → now, "HH:MM", "-Nm", "-NhMMm".
func ParseStartArg(arg string) (time.Time, error) {
	now := time.Now()
	if arg == "" {
		return now, nil
	}

	// HH:MM
	if len(arg) == 5 && arg[2] == ':' {
		var h, m int
		if _, err := fmt.Sscanf(arg, "%d:%d", &h, &m); err == nil {
			t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
			if t.After(now) {
				return time.Time{}, fmt.Errorf("zeit liegt in der Zukunft: %s", arg)
			}
			return t, nil
		}
	}

	// -NhMMm  or  -NhM  or  -Nh
	if len(arg) > 1 && arg[0] == '-' {
		rest := arg[1:]
		// find 'h'
		hi := strings.IndexByte(rest, 'h')
		if hi >= 0 {
			hStr := rest[:hi]
			mStr := strings.TrimSuffix(rest[hi+1:], "m")
			h, errH := strconv.Atoi(hStr)
			m := 0
			if mStr != "" {
				var errM error
				m, errM = strconv.Atoi(mStr)
				if errM != nil {
					return time.Time{}, fmt.Errorf("ungültig: %s", arg)
				}
			}
			if errH == nil {
				return now.Add(-time.Duration(h)*time.Hour - time.Duration(m)*time.Minute), nil
			}
		}
		// -Nm
		mStr := strings.TrimSuffix(rest, "m")
		if min, err := strconv.Atoi(mStr); err == nil {
			return now.Add(-time.Duration(min) * time.Minute), nil
		}
	}

	return time.Time{}, fmt.Errorf("unbekanntes Format: %s (erwartet: HH:MM, -1h30m, -45m)", arg)
}

// ParseStop parses a stop-time argument relative to a known start time:
//
//   - ""          → now
//   - "HH:MM"     → absolute time on the start's date (must be after start)
//   - "-Nh / -Nm" → now - offset (legacy, same as ParseStartArg)
//   - "+Nh / +Nm" → start + offset (duration shorthand: stop after N hours)
//
// The +Nh form is the reason this exists separately from ParseStartArg —
// "stop = start + 90m" is the most natural way to enter a session of known
// length, but ParseStartArg has no anchor to add to.
func ParseStop(arg string, start time.Time) (time.Time, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return time.Now(), nil
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
	return ParseStartArg(arg)
}

// parseHumanDuration parses "1h30m" / "90m" / "2h" / "45" (minutes) into a
// duration. Internal helper used by ParseStop.
func parseHumanDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("leer")
	}
	hi := strings.IndexByte(s, 'h')
	if hi >= 0 {
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

// — internal helpers —

func readActiveState(path string) (*time.Time, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return nil, err
	}
	t := time.Unix(epoch, 0)
	return &t, nil
}

func readSessions(path string, filter func(Session) bool) ([]Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var sessions []Session
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", parts[0], time.Local)
		if err != nil {
			continue
		}
		startHM, err := ParseHM(parts[1])
		if err != nil {
			continue
		}
		stopHM, err := ParseHM(parts[2])
		if err != nil {
			continue
		}
		elapsedSec, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		if err != nil {
			continue
		}
		base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
		s := Session{
			Date:    base,
			Start:   base.Add(startHM),
			Stop:    base.Add(stopHM),
			Elapsed: time.Duration(elapsedSec) * time.Second,
		}
		if len(parts) >= 5 {
			s.Tag = strings.TrimSpace(parts[4])
		}
		if len(parts) >= 6 {
			s.Note = strings.TrimSpace(parts[5])
		}
		if filter == nil || filter(s) {
			sessions = append(sessions, s)
		}
	}
	return sessions, sc.Err()
}

func appendLog(path string, s Session) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = writeSessionLine(f, s)
	return err
}

// writeSessionLine writes one TSV row. Trailing tag/note columns are omitted
// when empty so the file stays compact and forward-compatible readers see
// the historical 4-column shape unchanged.
func writeSessionLine(w io.Writer, s Session) (int, error) {
	base := fmt.Sprintf("%s\t%s\t%s\t%d",
		s.Date.Format("2006-01-02"),
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		int64(s.Elapsed.Seconds()),
	)
	if s.Tag != "" || s.Note != "" {
		base += "\t" + s.Tag
	}
	if s.Note != "" {
		base += "\t" + s.Note
	}
	return fmt.Fprintln(w, base)
}

// splitAtMidnight splits a span [start, stop) into one Session per calendar
// day so each day reflects its own elapsed time. Returns a single-element
// slice when the span doesn't cross midnight.
func splitAtMidnight(start, stop time.Time) []Session {
	if !sameDay(start, stop) && stop.After(start) {
		var parts []Session
		cur := start
		for {
			midnight := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location()).
				AddDate(0, 0, 1)
			end := midnight
			if !end.Before(stop) {
				end = stop
			}
			parts = append(parts, Session{
				Date:    time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location()),
				Start:   cur,
				Stop:    end,
				Elapsed: end.Sub(cur),
			})
			if !end.Before(stop) {
				break
			}
			cur = end
		}
		return parts
	}
	return []Session{{
		Date:    time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()),
		Start:   start,
		Stop:    stop,
		Elapsed: stop.Sub(start),
	}}
}

// withLock takes an exclusive flock on ~/.tmux/worktime.lock for the duration
// of fn. Concurrent CLI/TUI writers are serialized; readers are intentionally
// not locked since the per-second TUI tick would amplify contention and the
// log shape tolerates a stale read between writes.
func withLock(home string, fn func() error) error {
	dir := filepath.Join(home, ".tmux")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "worktime.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

// ParseHM parses an "HH:MM" string into a time.Duration offset from midnight.
func ParseHM(s string) (time.Duration, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid HH:MM: %s", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func tmuxOpt(name string) string {
	out, err := exec.Command("tmux", "show-options", "-gqv", "@"+name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
