// Package worktime implements the worktime screen and its state parser.
package worktime

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TargetHours is the daily work target (hours).
const TargetHours = 8

// Session represents a single completed worktime session.
type Session struct {
	Date    time.Time
	Start   time.Time
	Stop    time.Time
	Elapsed time.Duration
}

// Day holds all sessions for today plus an optional active session start.
type Day struct {
	Sessions []Session
	Active   *time.Time    // start time of the running session; nil if idle
	Logged   time.Duration // sum of completed sessions today
	Target   time.Duration
}

// Total returns the total tracked time for today: logged + current active elapsed.
// Active elapsed is capped at midnight so sessions that span days don't over-count.
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

// LoadToday reads ~/.tmux/worktime.state and ~/.tmux/worktime.log and
// returns a Day with all sessions for the current calendar day.
func LoadToday() (Day, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Day{Target: TargetHours * time.Hour}, err
	}
	base := filepath.Join(home, ".tmux")
	day := Day{Target: TargetHours * time.Hour}

	day.Active, _ = loadActiveState(filepath.Join(base, "worktime.state"))

	now := time.Now()
	sessions, err := loadTodaySessions(filepath.Join(base, "worktime.log"), now)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return day, err
	}
	day.Sessions = sessions
	for _, s := range sessions {
		day.Logged += s.Elapsed
	}
	return day, nil
}

// loadActiveState reads the epoch timestamp from worktime.state.
// Returns (nil, nil) when no session is active (file absent).
func loadActiveState(path string) (*time.Time, error) {
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

// loadTodaySessions parses worktime.log and returns only today's sessions.
// Log schema: DATE\tSTART_HM\tSTOP_HM\tELAPSED_SEC
func loadTodaySessions(path string, now time.Time) ([]Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	todayStr := now.Format("2006-01-02")
	loc := now.Location()

	var sessions []Session
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[0] != todayStr {
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
		base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		sessions = append(sessions, Session{
			Date:    base,
			Start:   base.Add(startHM),
			Stop:    base.Add(stopHM),
			Elapsed: time.Duration(elapsedSec) * time.Second,
		})
	}
	return sessions, sc.Err()
}

// parseHM parses "HH:MM" into a duration offset from midnight.
func parseHM(s string) (time.Duration, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, errors.New("invalid HH:MM: " + s)
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
