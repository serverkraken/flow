// Package worktime implements the worktime data layer: reading/writing
// ~/.tmux/worktime.state and ~/.tmux/worktime.log, plus week/history aggregation.
package worktime

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TargetHours is the daily work target.
const TargetHours = 8

// Session is a completed work session.
type Session struct {
	Date    time.Time
	Start   time.Time
	Stop    time.Time
	Elapsed time.Duration
}

// Day holds all sessions for today plus an optional active session.
type Day struct {
	Sessions []Session
	Active   *time.Time
	Logged   time.Duration
	Target   time.Duration
}

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
	home, err := os.UserHomeDir()
	if err != nil {
		return Day{Target: TargetHours * time.Hour}, err
	}
	base := filepath.Join(home, ".tmux")
	day := Day{Target: TargetHours * time.Hour}

	day.Active, _ = readActiveState(filepath.Join(base, "worktime.state"))

	now := time.Now()
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

// LoadWeek returns Mon–Fri of the current week.
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

	var week []WeekDay
	for i := 0; i < 5; i++ {
		day := monday.AddDate(0, 0, i)
		isToday := sameDay(day, now)

		sessions, _ := readSessions(logPath, func(s Session) bool {
			return sameDay(s.Date, day)
		})
		var logged time.Duration
		for _, s := range sessions {
			logged += s.Elapsed
		}

		var dayActive *time.Time
		if isToday {
			dayActive = active
		}

		week = append(week, WeekDay{
			Date:    day,
			Logged:  logged,
			Active:  dayActive,
			Target:  TargetHours * time.Hour,
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
			byDate[key] = &DayRecord{Date: s.Date, Target: TargetHours * time.Hour}
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

// Start writes the session start time to the state file.
func Start(ts time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(home, ".tmux", "worktime.state"),
		[]byte(strconv.FormatInt(ts.Unix(), 10)),
		0o644,
	)
}

// Stop stops the current session and logs it. Returns the completed session.
func Stop() (Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Session{}, err
	}
	statePath := filepath.Join(home, ".tmux", "worktime.state")
	logPath := filepath.Join(home, ".tmux", "worktime.log")

	active, err := readActiveState(statePath)
	if err != nil {
		return Session{}, err
	}
	if active == nil {
		return Session{}, errors.New("keine aktive Session")
	}

	now := time.Now()
	s := Session{
		Date:    time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		Start:   *active,
		Stop:    now,
		Elapsed: now.Sub(*active),
	}

	if err := appendLog(logPath, s); err != nil {
		return Session{}, err
	}
	return s, os.Remove(statePath)
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
	active, err := readActiveState(statePath)
	if err != nil || active == nil {
		return errors.New("keine aktive Session")
	}
	return os.WriteFile(statePath, []byte(strconv.FormatInt(ts.Unix(), 10)), 0o644)
}

// StopAt stops the current session with a specific stop time.
func StopAt(stopTime time.Time) (Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Session{}, err
	}
	statePath := filepath.Join(home, ".tmux", "worktime.state")
	logPath := filepath.Join(home, ".tmux", "worktime.log")

	active, err := readActiveState(statePath)
	if err != nil {
		return Session{}, err
	}
	if active == nil {
		return Session{}, errors.New("keine aktive Session")
	}
	if !stopTime.After(*active) {
		return Session{}, errors.New("Stoppzeit muss nach Startzeit liegen")
	}

	s := Session{
		Date:    time.Date(stopTime.Year(), stopTime.Month(), stopTime.Day(), 0, 0, 0, 0, stopTime.Location()),
		Start:   *active,
		Stop:    stopTime,
		Elapsed: stopTime.Sub(*active),
	}
	if err := appendLog(logPath, s); err != nil {
		return Session{}, err
	}
	return s, os.Remove(statePath)
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

// EditSession replaces the session at index idx (0-based) for the given date.
func EditSession(date time.Time, idx int, newStart, newStop time.Time) error {
	if !newStop.After(newStart) {
		return errors.New("Stoppzeit muss nach Startzeit liegen")
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
					}
				}
				dayIdx++
			}
		}
		return sessions
	})
}

// rewriteLog reads the entire log, applies fn, and writes it back atomically.
func rewriteLog(fn func([]Session) []Session) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, ".tmux", "worktime.log")

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
		if _, err := fmt.Fprintf(f, "%s\t%s\t%s\t%d\n",
			s.Date.Format("2006-01-02"),
			s.Start.Format("15:04"),
			s.Stop.Format("15:04"),
			int64(s.Elapsed.Seconds()),
		); err != nil {
			f.Close() //nolint:errcheck
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, logPath)
}

// AddManual appends a manual session entry to the log.
func AddManual(date, start, stop time.Time) error {
	if !stop.After(start) {
		return errors.New("Stop muss nach Start liegen")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	s := Session{
		Date:    date,
		Start:   start,
		Stop:    stop,
		Elapsed: stop.Sub(start),
	}
	return appendLog(filepath.Join(home, ".tmux", "worktime.log"), s)
}

// StatusSegment returns the tmux status-right segment string.
// Returns "" when nothing was tracked today.
func StatusSegment() string {
	green := tmuxOpt("tn_green")
	if green == "" {
		green = "#9ece6a"
	}
	yellow := tmuxOpt("tn_yellow")
	if yellow == "" {
		yellow = "#e0af68"
	}
	red := tmuxOpt("tn_red")
	if red == "" {
		red = "#f7768e"
	}
	dim := tmuxOpt("tn_dim")
	if dim == "" {
		dim = "#565f89"
	}

	now := time.Now()
	day, err := LoadToday()
	if err != nil {
		return ""
	}

	total := day.Total(now)
	if total == 0 {
		return ""
	}

	h := int(total.Hours())
	m := int(total.Minutes()) % 60

	if day.IsRunning() {
		target := time.Duration(TargetHours) * time.Hour
		var color string
		switch {
		case total >= target:
			color = red
		case total >= target-2*time.Hour:
			color = yellow
		default:
			color = green
		}
		return fmt.Sprintf("#[fg=%s,bold]⏱ %02d:%02d#[default]", color, h, m)
	}
	return fmt.Sprintf("#[fg=%s]⏸ %02d:%02d#[default]", dim, h, m)
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
				return time.Time{}, fmt.Errorf("Zeit liegt in der Zukunft: %s", arg)
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
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
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
		startHM, err := parseHM(parts[1])
		if err != nil {
			continue
		}
		stopHM, err := parseHM(parts[2])
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
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%d\n",
		s.Date.Format("2006-01-02"),
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		int64(s.Elapsed.Seconds()),
	)
	return err
}

func parseHM(s string) (time.Duration, error) {
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
