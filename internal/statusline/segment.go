package statusline

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Snapshot is everything BuildStatusSegment needs — built by the CLI from the
// worktimeStatusDTO plus the local clock, palette and MaxStreakMin.
type Snapshot struct {
	Now          time.Time
	LoggedMin    int // COMPLETED today, excl. the running session (client extrapolates)
	TargetMin    int // 0 = no target → idle-hit is trivially true
	Running      bool
	ActiveStart  time.Time // zero when !Running
	DayOff       *DayOffInfo
	Week         []WeekDay
	Streak       int
	SaldoMin     int // monthly burndown saldo
	SaldoTarget  int // burndown targetMin; 0 → no saldo marker
	Palette      StatusPalette
	MaxStreakMin int // active-session warning threshold (yellow at N, red at 2N); 0 disables
}

// DayOffInfo is today's day-off (banner). Kind drives the ● colour.
type DayOffInfo struct {
	Kind  domain.Kind
	Label string
}

// WeekDay is one Mon–Fri pace-dot input (from week[]).
type WeekDay struct {
	LoggedMin  int
	TargetMin  int
	Weekday    time.Weekday
	IsToday    bool
	DayOffKind domain.Kind // "" = none
}

const (
	bannerApproachingThreshold   = 2 * time.Hour // yellow "Endspurt" once target in sight
	bannerOvertimeAlertThreshold = 4 * time.Hour // red only on a truly excessive overrun
)

// BuildStatusSegment renders the tmux status-right string. "" when nothing was
// tracked today, no week activity exists and no day-off is set.
func BuildStatusSegment(in Snapshot) string {
	logged := time.Duration(in.LoggedMin) * time.Minute
	target := time.Duration(in.TargetMin) * time.Minute
	var tail time.Duration
	if in.Running && !in.ActiveStart.IsZero() {
		tail = clampedElapsed(in.Now, in.ActiveStart)
	}
	total := logged + tail
	dots := buildPaceDots(in.Week, in.Running, in.Palette)
	// Empty only when NOTHING is happening: no time today, no week activity, no
	// day-off AND no running timer. The !in.Running guard is a deliberate
	// deviation from the old port (Finding #7): a session started on a weekend
	// at the very first tick (elapsed≈0, dots="" because weekends are skipped)
	// would otherwise blank a genuinely running timer. A running timer is
	// "tracked" (Spec §2 empty-criteria), so it must render.
	if total == 0 && dots == "" && in.DayOff == nil && !in.Running {
		return ""
	}
	achieved := target == 0 || total >= target
	icon, mainAttr := statusBanner(in.Running, total, target, achieved, in.Palette)

	var parts []string
	if in.DayOff != nil {
		parts = append(parts, fmt.Sprintf("#[fg=%s]● %s#[default]",
			KindStatusColor(in.DayOff.Kind, in.Palette), in.DayOff.Label))
	}
	parts = append(parts, fmt.Sprintf("#[fg=%s]%s %02d:%02d#[default]",
		mainAttr, icon, int(total.Hours()), int(total.Minutes())%60))
	if in.Running && !in.ActiveStart.IsZero() {
		parts = append(parts, activeSessionParts(in, logged, target, achieved)...)
	}
	if achieved && total > 0 {
		parts = append(parts, fmt.Sprintf("#[fg=%s,bold]✓#[default]", in.Palette.Green))
	}
	if dots != "" {
		parts = append(parts, dots)
	}
	if in.Streak >= 3 {
		parts = append(parts, fmt.Sprintf("#[fg=%s]Streak %d#[default]", in.Palette.Green, in.Streak))
	}
	parts = append(parts, monthBurndownPart(in.SaldoMin, in.SaldoTarget, in.Palette)...)
	return strings.Join(parts, " ")
}

// clampedElapsed is (now - start) with start floored to today's midnight and a
// negative result floored to 0 — a session started yesterday reports only
// today's portion; a start in the future reports 0.
func clampedElapsed(now, start time.Time) time.Duration {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if start.Before(midnight) {
		start = midnight
	}
	if e := now.Sub(start); e > 0 {
		return e
	}
	return 0
}

func statusBanner(running bool, total, target time.Duration, achieved bool, p StatusPalette) (icon, attr string) {
	if running {
		switch {
		case total >= target+bannerOvertimeAlertThreshold:
			return "⏱", p.Red + ",bold"
		case achieved:
			return "⏱", p.Green + ",bold"
		case total >= target-bannerApproachingThreshold:
			return "⏱", p.Yellow + ",bold"
		default:
			return "⏱", p.Cyan + ",bold"
		}
	}
	if achieved && total > 0 {
		return "‖", p.Green
	}
	return "‖", p.Dim
}

func activeSessionParts(in Snapshot, logged, target time.Duration, achieved bool) []string {
	midnight := time.Date(in.Now.Year(), in.Now.Month(), in.Now.Day(), 0, 0, 0, 0, in.Now.Location())
	start := in.ActiveStart
	if start.Before(midnight) {
		start = midnight
	}
	elapsed := in.Now.Sub(start)
	if elapsed < 0 {
		elapsed = 0
	}
	streakColor, glyph := in.Palette.Dim, "▶"
	minutes := int(elapsed.Minutes())
	switch {
	case in.MaxStreakMin > 0 && minutes >= 2*in.MaxStreakMin:
		streakColor, glyph = in.Palette.Red, "▶!"
	case in.MaxStreakMin > 0 && minutes >= in.MaxStreakMin:
		streakColor, glyph = in.Palette.Yellow, "▶!"
	}
	out := []string{fmt.Sprintf("#[fg=%s]%s %d:%02d#[default]",
		streakColor, glyph, int(elapsed.Hours()), int(elapsed.Minutes())%60)}
	if !achieved {
		etaT := start.Add(target - logged) // same clamped start as elapsed
		out = append(out, fmt.Sprintf("#[fg=%s]→%s#[default]", in.Palette.Dim, etaT.Format("15:04")))
	}
	return out
}

// monthBurndownPart renders ▲/▼ monthly saldo. Nothing when |saldo| < 1h or no
// target. Hours are ROUNDED (a 1h59m surplus is "▲ +2h", not "▲ +1h").
func monthBurndownPart(saldoMin, targetMin int, p StatusPalette) []string {
	if targetMin == 0 {
		return nil
	}
	saldo := time.Duration(saldoMin) * time.Minute
	const min = time.Hour
	switch {
	case saldo >= min:
		return []string{fmt.Sprintf("#[fg=%s]▲ +%dh#[default]", p.Green, int(saldo.Round(time.Hour).Hours()))}
	case saldo <= -min:
		return []string{fmt.Sprintf("#[fg=%s]▼ -%dh#[default]", p.Yellow, int((-saldo).Round(time.Hour).Hours()))}
	}
	return nil
}
