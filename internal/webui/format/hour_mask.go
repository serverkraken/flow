package format

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// HourMask returns a 24-element array where mask[h]=1 if any session in
// `sessions` overlaps the [h:00, h+1:00) window of `now`'s local day, or
// if `active` started during that hour (or earlier today and the hour is
// ≤ now's hour). Hours outside today's range are 0.
//
// A session that crosses an hour boundary marks every hour it touches.
func HourMask(sessions []domain.Session, active *time.Time, now time.Time) [24]int {
	var mask [24]int
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)

	for _, s := range sessions {
		start := s.Start
		stop := s.Stop
		if !stop.After(dayStart) || !start.Before(dayEnd) {
			continue
		}
		if start.Before(dayStart) {
			start = dayStart
		}
		if stop.After(dayEnd) {
			stop = dayEnd
		}
		for h := start.Hour(); h <= stop.Hour() && h < 24; h++ {
			// A session that ends exactly at hour boundary (stop.Minute=0 and
			// stop.Second=0) should not mark the boundary hour as worked.
			if h == stop.Hour() && stop.Minute() == 0 && stop.Second() == 0 && h != start.Hour() {
				continue
			}
			mask[h] = 1
		}
	}

	if active != nil {
		start := *active
		if start.Before(dayStart) {
			start = dayStart
		}
		if !start.Before(dayEnd) {
			return mask
		}
		for h := start.Hour(); h <= now.Hour() && h < 24; h++ {
			mask[h] = 1
		}
	}
	return mask
}
