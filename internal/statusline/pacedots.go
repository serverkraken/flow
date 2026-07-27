package statusline

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type paceDotKind int

const (
	paceMissed paceDotKind = iota
	paceHit
	paceRunning
	paceDayOff
)

// buildPaceDots renders Mon–Fri dots. "" when no weekday has any accounted slot
// (all missed) — avoids a stray dim row at the start of an empty week.
func buildPaceDots(week []WeekDay, running bool, p StatusPalette) string {
	var parts []string
	any := false
	for _, d := range week {
		if d.Weekday == time.Saturday || d.Weekday == time.Sunday {
			continue
		}
		k := classify(d, running)
		if k != paceMissed {
			any = true
		}
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s#[default]", paceColor(k, d.DayOffKind, p), glyph(k)))
	}
	if !any {
		return ""
	}
	return strings.Join(parts, "")
}

func classify(d WeekDay, running bool) paceDotKind {
	if d.DayOffKind != "" {
		return paceDayOff
	}
	if d.TargetMin > 0 && d.LoggedMin >= d.TargetMin {
		return paceHit
	}
	if d.IsToday && running {
		return paceRunning
	}
	return paceMissed
}

func glyph(k paceDotKind) string {
	if k == paceMissed {
		return "○" // ○
	}
	return "●" // ●
}

func paceColor(k paceDotKind, kind domain.Kind, p StatusPalette) string {
	switch k {
	case paceHit:
		return p.Green
	case paceRunning:
		return p.Cyan
	case paceDayOff:
		return KindStatusColor(kind, p)
	}
	return p.Dim
}

// KindStatusColor maps a day-off Kind onto a palette slot: Holiday→Blue
// (info/scheduled), Vacation→Purple (identity), Sick→Orange (pending);
// every other kind → Dim. Ported from the old domain/status.go.
func KindStatusColor(k domain.Kind, p StatusPalette) string {
	switch k {
	case domain.KindHoliday:
		return p.Blue
	case domain.KindVacation:
		return p.Purple
	case domain.KindSick:
		return p.Orange
	}
	return p.Dim
}
