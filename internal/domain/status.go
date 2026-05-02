package domain

import (
	"fmt"
	"strings"
	"time"
)

// StatusPalette is the colour set used by tmux #[fg=...] markers in the
// status-right segment. Hex codes match the tokyonight defaults flow ships.
type StatusPalette struct {
	Green, Yellow, Red, Cyan, Dim string
}

// DefaultStatusPalette returns the tokyonight defaults. Adapters that read
// tmux options (`@tn_green`, `@tn_yellow`, …) override these per call.
func DefaultStatusPalette() StatusPalette {
	return StatusPalette{
		Green:  "#9ece6a",
		Yellow: "#e0af68",
		Red:    "#f7768e",
		Cyan:   "#7dcfff",
		Dim:    "#565f89",
	}
}

// StatusInputs is everything BuildStatusSegment needs to render the
// tmux status-right segment. The pure renderer takes a snapshot —
// callers (use cases) gather the data from their adapters once per tick.
type StatusInputs struct {
	Now      time.Time
	Day      Day
	Week     []WeekDay
	DayOff   *DayOff             // today's dayoff entry, nil when not configured
	Target   time.Duration       // TargetFor(today)
	Streak   int                 // current workday streak; rendered when ≥ 3
	Burndown MonthBurndownReport // monthly saldo light
	// LookupDayOff returns the dayoff entry for a given date, used by the
	// pace dots to render free days as ★/☼/✚ instead of a missed-target ○.
	LookupDayOff func(time.Time) (DayOff, bool)
	Palette      StatusPalette
	// MaxStreakMin is the active-session threshold (yellow at MaxStreakMin,
	// red at 2×). 0 disables the colour shift entirely.
	MaxStreakMin int
}

// BuildStatusSegment renders the tmux status-right segment string. Returns
// "" when nothing was tracked today and no week activity exists.
//
// Layout: [Frei: …] ⏱ HH:MM ▶ S:MM →HH:MM ✓ ●●●●○ Streak N ▲ +Nh
func BuildStatusSegment(in StatusInputs) string {
	total := in.Day.Total(in.Now)
	dots := BuildPaceDots(in.Week, in.Now, in.LookupDayOff, in.Palette)

	if total == 0 && dots == "" && in.DayOff == nil {
		return ""
	}

	achieved := in.Target == 0 || total >= in.Target
	icon, mainAttr := statusBanner(in.Day, total, in.Target, achieved, in.Palette)

	var parts []string
	if in.DayOff != nil {
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s %s#[default]",
			in.Palette.Cyan, bannerDayOffGlyph(in.DayOff.Kind), in.DayOff.Label))
	}
	parts = append(parts, fmt.Sprintf("#[fg=%s]%s %02d:%02d#[default]",
		mainAttr, icon, int(total.Hours()), int(total.Minutes())%60))

	if in.Day.IsRunning() && in.Day.Active != nil {
		parts = append(parts, activeSessionParts(in, total, achieved)...)
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
	parts = append(parts, monthBurndownPart(in.Burndown, in.Palette)...)

	return strings.Join(parts, " ")
}

// statusBanner picks the icon + tmux attr string for the main HH:MM banner.
// Bold + warm colour when running, dim when idle. Red is reserved for
// "really a lot" — normal +1h overtime stays green.
func statusBanner(day Day, total, target time.Duration, achieved bool, pal StatusPalette) (icon, attr string) {
	if day.IsRunning() {
		switch {
		case total >= target+4*time.Hour:
			return "⏱", pal.Red + ",bold"
		case achieved:
			return "⏱", pal.Green + ",bold"
		case total >= target-2*time.Hour:
			return "⏱", pal.Yellow + ",bold"
		default:
			return "⏱", pal.Cyan + ",bold"
		}
	}
	if achieved && total > 0 {
		return "⏸", pal.Green
	}
	return "⏸", pal.Dim
}

// activeSessionParts renders the "▶ S:MM" running-session indicator and
// the "→HH:MM" projected target ETA when below target.
func activeSessionParts(in StatusInputs, total time.Duration, achieved bool) []string {
	elapsed := in.Now.Sub(*in.Day.Active)
	if elapsed < 0 {
		elapsed = 0
	}
	streakColor := in.Palette.Dim
	minutes := int(elapsed.Minutes())
	// `!` glyph signals "you've been at it a while" in peripheral vision.
	// Colour shift alone is easy to miss when the eye scans past; the
	// extra char makes the warning unambiguous.
	glyph := "▶"
	switch {
	case in.MaxStreakMin > 0 && minutes >= 2*in.MaxStreakMin:
		streakColor = in.Palette.Red
		glyph = "▶!"
	case in.MaxStreakMin > 0 && minutes >= in.MaxStreakMin:
		streakColor = in.Palette.Yellow
		glyph = "▶!"
	}
	out := []string{fmt.Sprintf("#[fg=%s]%s %d:%02d#[default]",
		streakColor, glyph, int(elapsed.Hours()), int(elapsed.Minutes())%60)}

	if !achieved {
		etaT := in.Day.Active.Add(in.Target - in.Day.Logged)
		out = append(out, fmt.Sprintf("#[fg=%s]→%s#[default]",
			in.Palette.Dim, etaT.Format("15:04")))
	}
	// total is referenced by the caller — keep the parameter for future use.
	_ = total
	return out
}

// monthBurndownPart renders the ▲/▼ monthly saldo indicator. Returns nothing
// when |saldo| < 1h so trivial 15-minute fluctuations don't flicker the row.
func monthBurndownPart(rep MonthBurndownReport, pal StatusPalette) []string {
	if rep.Target == 0 {
		return nil
	}
	const min = time.Hour
	switch {
	case rep.Saldo >= min:
		h := int(rep.Saldo.Hours())
		return []string{fmt.Sprintf("#[fg=%s]▲ +%dh#[default]", pal.Green, h)}
	case rep.Saldo <= -min:
		h := int((-rep.Saldo).Hours())
		return []string{fmt.Sprintf("#[fg=%s]▼ -%dh#[default]", pal.Yellow, h)}
	}
	return nil
}

// BuildPaceDots renders Mon–Fri pace dots. Returns "" when no weekday in
// the week has any activity — avoids a stray segment at the start of an
// empty week. Day-offs (Feiertag/Urlaub/Krank) get a distinct glyph + cyan
// tint so the row says "I had a free day" instead of "I missed a target".
func BuildPaceDots(
	week []WeekDay, now time.Time,
	lookup func(time.Time) (DayOff, bool),
	pal StatusPalette,
) string {
	type dot struct{ ch, color string }
	var dots []dot
	any := false
	for _, d := range week {
		if d.Date.Weekday() == time.Saturday || d.Date.Weekday() == time.Sunday {
			continue
		}
		if lookup != nil {
			if dayOff, isOff := lookup(d.Date); isOff {
				dots = append(dots, dot{dotDayOffGlyph(dayOff.Kind), pal.Cyan})
				any = true
				continue
			}
		}
		total := d.Total(now)
		switch {
		case total >= d.Target:
			dots = append(dots, dot{"●", pal.Green})
			any = true
		case d.IsToday && d.Active != nil:
			// Same glyph as "hit", colour-differentiated. Half-fill glyphs
			// like ◐ render at emoji width in some fonts and break alignment.
			dots = append(dots, dot{"●", pal.Yellow})
			any = true
		default:
			dots = append(dots, dot{"○", pal.Dim})
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

// bannerDayOffGlyph picks a monospace TUI marker for each kind in the
// "[Frei: …]" banner. Default "·" when the kind is unknown.
func bannerDayOffGlyph(k Kind) string {
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

// dotDayOffGlyph mirrors bannerDayOffGlyph but with "○" as the unknown-
// kind fallback so the pace-dots row keeps its column rhythm.
func dotDayOffGlyph(k Kind) string {
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
