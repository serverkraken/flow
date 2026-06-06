package format

import (
	"fmt"
	"time"
)

// HumanRelativeTime renders `t` relative to `now` in a humane German
// format ("heute · 09:28", "gestern · 17:45", "vor 2 Tagen · 14:12",
// "vor 8s"). Sub-minute differences emit "vor Ns" / "vor Nm" so the
// activity stream's "letzte Aktivität" card has a useful readout for
// fresh events.
func HumanRelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = 0
	}
	if delta < time.Minute {
		secs := int(delta / time.Second)
		if secs < 1 {
			secs = 1
		}
		return fmt.Sprintf("vor %ds", secs)
	}
	if delta < time.Hour {
		mins := int(delta / time.Minute)
		return fmt.Sprintf("vor %dm", mins)
	}

	tDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(nowDay.Sub(tDay) / (24 * time.Hour))
	hm := t.Format("15:04")
	switch days {
	case 0:
		return fmt.Sprintf("heute · %s", hm)
	case 1:
		return fmt.Sprintf("gestern · %s", hm)
	default:
		return fmt.Sprintf("vor %d Tagen · %s", days, hm)
	}
}
