package worktime

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatDurLive(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm %02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func pctOfTarget(total, target time.Duration) int {
	if target <= 0 {
		return 0
	}
	pct := int(total * 100 / target)
	if pct > 100 {
		pct = 100
	}
	return pct
}

var (
	durationWidth8Style = lipgloss.NewStyle().Width(8)
	boldStyle           = lipgloss.NewStyle().Bold(true)
	fgStyle             = lipgloss.NewStyle()
)

var deWeekday = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

func fmtDateDe(t time.Time) string {
	return fmt.Sprintf("%s · %02d.%02d.%04d", deWeekday[int(t.Weekday())], t.Day(), int(t.Month()), t.Year())
}

func parseHM(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("invalid HH:MM %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

func normalizeDurationArg(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "+")
}

func parseStop(arg string, start, _ time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(arg); err == nil {
		return start.Add(d), nil
	}
	hm, err := parseHM(arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid stop %q", arg)
	}
	base := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	return base.Add(hm), nil
}
