package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/statusline"
)

func pal() statusline.StatusPalette { return statusline.DefaultStatusPalette() }

func TestBuildStatusSegment_EmptyDayReturnsEmpty(t *testing.T) {
	in := statusline.Snapshot{
		Now: time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local), TargetMin: 480, Palette: pal(),
	}
	if got := statusline.BuildStatusSegment(in); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestBuildStatusSegment_IdleHit(t *testing.T) {
	in := statusline.Snapshot{
		Now:       time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local),
		LoggedMin: 480, TargetMin: 480, Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "‖ 08:00") || !strings.Contains(got, "✓") {
		t.Errorf("idle hit should show '‖ 08:00' + '✓': %q", got)
	}
}

func TestBuildStatusSegment_IdleMissed(t *testing.T) {
	in := statusline.Snapshot{
		Now:       time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local),
		LoggedMin: 240, TargetMin: 480, Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "‖") || strings.Contains(got, "✓") {
		t.Errorf("missed day should be ‖ without ✓: %q", got)
	}
}

func TestBuildStatusSegment_RunningColors(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	mk := func(loggedMin int, sessionLen time.Duration) statusline.Snapshot {
		return statusline.Snapshot{
			Now: now, LoggedMin: loggedMin, TargetMin: 480, Running: true,
			ActiveStart: now.Add(-sessionLen), Palette: pal(),
		}
	}
	tests := []struct {
		name      string
		in        statusline.Snapshot
		mainColor string
	}{
		{"way over (red)", mk(13*60, time.Minute), pal().Red},
		{"hit (green)", mk(8*60, time.Minute), pal().Green},
		{"close (yellow)", mk(7*60, time.Minute), pal().Yellow},
		{"far (cyan)", mk(60, time.Minute), pal().Cyan},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := statusline.BuildStatusSegment(tc.in)
			if !strings.Contains(got, tc.mainColor) {
				t.Errorf("expected colour %s in %q", tc.mainColor, got)
			}
		})
	}
}

func TestBuildStatusSegment_RunningWayOverIsRed(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 13 * 60, TargetMin: 480, Running: true,
		ActiveStart: now.Add(-time.Minute), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, pal().Red) {
		t.Errorf("13h with 8h target should render red:\n%s", got)
	}
}

func TestBuildStatusSegment_LongRunningStreakWarning(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	mk := func(sessionLen time.Duration, maxStreak int) statusline.Snapshot {
		return statusline.Snapshot{
			Now: now, TargetMin: 480, Running: true,
			ActiveStart: now.Add(-sessionLen), MaxStreakMin: maxStreak, Palette: pal(),
		}
	}

	t.Run("under threshold has no warning glyph", func(t *testing.T) {
		if got := statusline.BuildStatusSegment(mk(30*time.Minute, 90)); strings.Contains(got, "▶!") {
			t.Errorf("30 min should not render ▶!, got: %q", got)
		}
	})
	t.Run("over threshold yellow ▶!", func(t *testing.T) {
		got := statusline.BuildStatusSegment(mk(100*time.Minute, 90))
		if !strings.Contains(got, "▶!") {
			t.Errorf("100 min should render ▶!: %q", got)
		}
		if !strings.Contains(got, pal().Yellow) {
			t.Errorf("expected yellow at 100 min: %q", got)
		}
	})
	t.Run("over 2x threshold red ▶!", func(t *testing.T) {
		got := statusline.BuildStatusSegment(mk(200*time.Minute, 90))
		if !strings.Contains(got, "▶!") {
			t.Errorf("200 min should render ▶!: %q", got)
		}
		if !strings.Contains(got, pal().Red) {
			t.Errorf("expected red at 200 min: %q", got)
		}
	})
	t.Run("MaxStreakMin=0 disables warning", func(t *testing.T) {
		if got := statusline.BuildStatusSegment(mk(200*time.Minute, 0)); strings.Contains(got, "▶!") {
			t.Errorf("MaxStreakMin=0 should not render ▶!, got: %q", got)
		}
	})
}

func TestBuildStatusSegment_RunningExcludesLiveTailFromLoggedButBankerIncludesIt(t *testing.T) {
	// loggedMin = completed only (240). ActiveStart 30 min ago. Banner total = 270 min = 4:30.
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 240, TargetMin: 480, Running: true,
		ActiveStart: now.Add(-30 * time.Minute), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "⏱ 04:30") { // banner = logged+tail
		t.Errorf("banner should extrapolate tail to 04:30: %q", got)
	}
	if !strings.Contains(got, "▶ 0:30") { // running session marker
		t.Errorf("running marker should be 0:30: %q", got)
	}
}

func TestBuildStatusSegment_ETAWhenRunningBelowTarget(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 240, TargetMin: 480, Running: true,
		ActiveStart: now.Add(-30 * time.Minute), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	// active.Add(target - logged) = 14:00 + (8h - 4h) = 18:00.
	if !strings.Contains(got, "→18:00") {
		t.Errorf("ETA should be 18:00: %q", got)
	}
}

func TestBuildStatusSegment_ETACrossesMidnight(t *testing.T) {
	now := time.Date(2026, 4, 29, 6, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 0, TargetMin: 480, Running: true,
		ActiveStart: time.Date(2026, 4, 28, 22, 0, 0, 0, time.Local), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "→08:00") || strings.Contains(got, "→06:00") {
		t.Errorf("ETA must clamp to today midnight (→08:00), not →06:00: %q", got)
	}
}

func TestBuildStatusSegment_NegativeElapsedClampsToZero(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, TargetMin: 480, Running: true,
		ActiveStart: now.Add(time.Hour), Palette: pal(), // future start
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "▶ 0:00") {
		t.Errorf("expected ▶ 0:00 for clamped elapsed, got: %q", got)
	}
}

func TestBuildStatusSegment_DayOffBanner(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, TargetMin: 0, Palette: pal(),
		DayOff: &statusline.DayOffInfo{Kind: domain.KindHoliday, Label: "Tag der Arbeit"},
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "● Tag der Arbeit") {
		t.Errorf("dayoff banner missing: %q", got)
	}
}

func TestBuildStatusSegment_DayOffBannerGlyphPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	p := pal()
	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, p.Blue},
		{domain.KindVacation, p.Purple},
		{domain.KindSick, p.Orange},
		{domain.Kind("unknown"), p.Dim},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			in := statusline.Snapshot{
				Now: now, TargetMin: 0, Palette: p,
				DayOff: &statusline.DayOffInfo{Kind: tc.kind, Label: "Test"},
			}
			got := statusline.BuildStatusSegment(in)
			want := tc.color + "]● Test"
			if !strings.Contains(got, want) {
				t.Errorf("kind %q expected %q in banner: %q", tc.kind, want, got)
			}
		})
	}
}

func TestBuildStatusSegment_DayOffOnlyStillRenders(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, TargetMin: 0, Palette: pal(),
		DayOff: &statusline.DayOffInfo{Kind: domain.KindVacation, Label: "Brückentag"},
	}
	got := statusline.BuildStatusSegment(in)
	if got == "" {
		t.Error("dayoff alone should produce a segment")
	}
	if !strings.Contains(got, "Brückentag") {
		t.Errorf("dayoff banner missing label: %q", got)
	}
}

func TestBuildStatusSegment_StreakRendersAt3(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	t.Run("streak < 3 not shown", func(t *testing.T) {
		in := statusline.Snapshot{Now: now, LoggedMin: 480, TargetMin: 480, Streak: 2, Palette: pal()}
		if strings.Contains(statusline.BuildStatusSegment(in), "Streak") {
			t.Error("streak 2 should not show")
		}
	})
	t.Run("streak >= 3 shown", func(t *testing.T) {
		in := statusline.Snapshot{Now: now, LoggedMin: 480, TargetMin: 480, Streak: 5, Palette: pal()}
		if !strings.Contains(statusline.BuildStatusSegment(in), "Streak 5") {
			t.Error("streak 5 should render")
		}
	})
}

func TestBuildStatusSegment_BurndownArrows(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	tests := []struct {
		name              string
		saldoMin, targetM int
		want              string
	}{
		{"on track over 1h", 5 * 60, 160 * 60, "▲ +5h"},
		{"under 1h shows nothing", 30, 160 * 60, ""},
		{"under -1h", -3 * 60, 160 * 60, "▼ -3h"},
		{"target=0 shows nothing", 5 * 60, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := statusline.Snapshot{
				Now: now, LoggedMin: 480, TargetMin: 480,
				SaldoMin: tc.saldoMin, SaldoTarget: tc.targetM, Palette: pal(),
			}
			got := statusline.BuildStatusSegment(in)
			if tc.want == "" {
				if strings.Contains(got, "▲") || strings.Contains(got, "▼") {
					t.Errorf("expected no arrow, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("missing %q in %q", tc.want, got)
			}
		})
	}
}

func TestBuildStatusSegment_DotsAttached(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 30, TargetMin: 480, Palette: pal(),
		Week: []statusline.WeekDay{
			{Weekday: time.Monday, TargetMin: 480, LoggedMin: 480}, // hit
			{Weekday: time.Wednesday, TargetMin: 480, LoggedMin: 0, IsToday: true},
		},
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "●") {
		t.Errorf("dots should be attached, got %q", got)
	}
}

func TestBuildStatusSegment_DimmedPaletteAllSlotsDim(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	base := pal()
	in := statusline.Snapshot{
		Now: now, LoggedMin: 480, TargetMin: 480, Streak: 5, Palette: base.Dimmed(),
	}
	got := statusline.BuildStatusSegment(in)
	if strings.Contains(got, base.Green) || strings.Contains(got, base.Cyan) {
		t.Errorf("dimmed render must use no live colours: %q", got)
	}
	if !strings.Contains(got, base.Dim) {
		t.Errorf("dimmed render should carry the Dim colour: %q", got)
	}
}

// A session started at the very first tick (elapsed≈0, empty week) must still
// render — a running timer is "tracked" (Finding #7).
func TestBuildStatusSegment_RunningAtZeroRendersNonEmpty(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.Local) // Saturday
	in := statusline.Snapshot{
		Now: now, LoggedMin: 0, TargetMin: 480, Running: true,
		ActiveStart: now, Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if got == "" {
		t.Fatal("running timer at zero must not render empty")
	}
	if !strings.Contains(got, "▶ 0:00") {
		t.Errorf("expected ▶ 0:00, got %q", got)
	}
}
