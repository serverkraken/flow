package domain

import "time"

// PaceDotKind is the semantic slot a per-weekday pace dot represents.
// Shared between the tmux status segment (BuildPaceDots) and the
// in-app week pace strip (worktime/week.renderPace) so the decision
// tree lives once. Each caller maps the kind to its own render layer
// (tmux #[fg=…] markers vs lipgloss styles).
//
// Glyph carries shape-of-status (PaceDotGlyph), colour carries kind:
//
//	PaceDotHit       ● in Success         workday with target met
//	PaceDotRunning   ● in Active (Cyan)   today, live session, target not yet met
//	PaceDotDayOff    ● in kind colour     scheduled free day — colour from KindStatusColor / theme.KindColor
//	PaceDotMissed    ○ in Dim             workday open / future / missed (default)
//
// Weekends are not represented here — the caller skips them before
// classifying. Likewise "no activity at all" suppression (the empty-
// week guard in BuildPaceDots) stays at the call site, because the
// renderer also wants to know *any-activity* for output suppression.
type PaceDotKind int

const (
	PaceDotMissed PaceDotKind = iota
	PaceDotHit
	PaceDotRunning
	PaceDotDayOff
)

// PaceDotFor classifies a single workday WeekDay into one of the four
// PaceDotKind slots. dayOff is non-nil when the caller's lookup has
// already resolved a DayOff entry for d.Date — passing the resolved
// pointer instead of a lookup closure keeps this function pure and
// trivially testable.
func PaceDotFor(d WeekDay, now time.Time, dayOff *DayOff) PaceDotKind {
	if dayOff != nil {
		return PaceDotDayOff
	}
	total := d.Total(now)
	if d.Target > 0 && total >= d.Target {
		return PaceDotHit
	}
	if d.IsToday && d.Active != nil {
		return PaceDotRunning
	}
	return PaceDotMissed
}

// PaceDotGlyph returns the canonical glyph for a pace dot kind. Only
// PaceDotMissed uses the outlined ○; every accounted-for slot uses ●
// so the colour signal stays strong at status-bar font sizes (Spec
// 2026-05-13-filled-dayoff-dots-supersede).
func PaceDotGlyph(k PaceDotKind) string {
	if k == PaceDotMissed {
		return "○"
	}
	return "●"
}
