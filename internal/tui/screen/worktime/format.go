package worktime

import (
	"fmt"
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

