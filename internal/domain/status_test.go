package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func pal() domain.StatusPalette { return domain.DefaultStatusPalette() }

// noLookup is a stand-in for "no day-offs configured".
func noLookup(time.Time) (domain.DayOff, bool) { return domain.DayOff{}, false }

func TestBuildStatusSegment_EmptyDayReturnsEmpty(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := domain.StatusInputs{
		Now:          now,
		Day:          domain.Day{Target: 8 * time.Hour},
		Target:       8 * time.Hour,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	if got := domain.BuildStatusSegment(in); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestBuildStatusSegment_IdleHit(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	in := domain.StatusInputs{
		Now:          now,
		Day:          domain.Day{Logged: 8 * time.Hour, Target: 8 * time.Hour},
		Target:       8 * time.Hour,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, "‖ 08:00") {
		t.Errorf("missing idle banner with 08:00: %q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("missing ✓ for hit day: %q", got)
	}
}

func TestBuildStatusSegment_IdleMissed(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	in := domain.StatusInputs{
		Now:          now,
		Day:          domain.Day{Logged: 4 * time.Hour, Target: 8 * time.Hour},
		Target:       8 * time.Hour,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, "‖") || strings.Contains(got, "✓") {
		t.Errorf("missed day should be ‖ without ✓: %q", got)
	}
}

func TestBuildStatusSegment_RunningColors(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	target := 8 * time.Hour
	mk := func(logged, sessionLen time.Duration) domain.StatusInputs {
		start := now.Add(-sessionLen)
		return domain.StatusInputs{
			Now:          now,
			Day:          domain.Day{Logged: logged, Active: &start, Target: target},
			Target:       target,
			LookupDayOff: noLookup,
			Palette:      pal(),
		}
	}

	tests := []struct {
		name      string
		in        domain.StatusInputs
		mainColor string
	}{
		// Red triggers at total >= target+4h. 13h logged + ~0 active = 13h.
		{"way over (red)", mk(13*time.Hour, time.Minute), pal().Red},
		{"hit (green)", mk(8*time.Hour, time.Minute), pal().Green},
		{"close (yellow)", mk(7*time.Hour, time.Minute), pal().Yellow},
		{"far (cyan)", mk(time.Hour, time.Minute), pal().Cyan},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.BuildStatusSegment(tc.in)
			if !strings.Contains(got, tc.mainColor) {
				t.Errorf("expected colour %s in %q", tc.mainColor, got)
			}
		})
	}
}

func TestBuildStatusSegment_RunningWayOverIsRed(t *testing.T) {
	// total = logged + active tail. target+4h boundary.
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.Local)
	start := now.Add(-time.Minute)
	target := 8 * time.Hour
	in := domain.StatusInputs{
		Now:          now,
		Day:          domain.Day{Logged: 13 * time.Hour, Active: &start, Target: target}, // 13h logged, ~13h total
		Target:       target,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, pal().Red) {
		t.Errorf("13h with 8h target should render red:\n%s", got)
	}
}

func TestBuildStatusSegment_LongRunningStreakWarning(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	target := 8 * time.Hour

	t.Run("under threshold has no warning glyph", func(t *testing.T) {
		start := now.Add(-30 * time.Minute)
		in := domain.StatusInputs{
			Now: now, Day: domain.Day{Active: &start, Target: target},
			Target: target, MaxStreakMin: 90, Palette: pal(),
			LookupDayOff: noLookup,
		}
		got := domain.BuildStatusSegment(in)
		if strings.Contains(got, "▶!") {
			t.Errorf("30 min should not render ▶!, got: %q", got)
		}
	})

	t.Run("over threshold yellow ▶!", func(t *testing.T) {
		start := now.Add(-100 * time.Minute)
		in := domain.StatusInputs{
			Now: now, Day: domain.Day{Active: &start, Target: target},
			Target: target, MaxStreakMin: 90, Palette: pal(),
			LookupDayOff: noLookup,
		}
		got := domain.BuildStatusSegment(in)
		if !strings.Contains(got, "▶!") {
			t.Errorf("100 min should render ▶!: %q", got)
		}
		if !strings.Contains(got, pal().Yellow) {
			t.Errorf("expected yellow at 100 min: %q", got)
		}
	})

	t.Run("over 2x threshold red ▶!", func(t *testing.T) {
		start := now.Add(-200 * time.Minute)
		in := domain.StatusInputs{
			Now: now, Day: domain.Day{Active: &start, Target: target},
			Target: target, MaxStreakMin: 90, Palette: pal(),
			LookupDayOff: noLookup,
		}
		got := domain.BuildStatusSegment(in)
		if !strings.Contains(got, "▶!") {
			t.Errorf("200 min should render ▶!: %q", got)
		}
		if !strings.Contains(got, pal().Red) {
			t.Errorf("expected red at 200 min: %q", got)
		}
	})

	t.Run("MaxStreakMin=0 disables warning", func(t *testing.T) {
		start := now.Add(-200 * time.Minute)
		in := domain.StatusInputs{
			Now: now, Day: domain.Day{Active: &start, Target: target},
			Target: target, MaxStreakMin: 0, Palette: pal(),
			LookupDayOff: noLookup,
		}
		got := domain.BuildStatusSegment(in)
		if strings.Contains(got, "▶!") {
			t.Errorf("MaxStreakMin=0 should not render ▶!, got: %q", got)
		}
	})
}

func TestBuildStatusSegment_ETAWhenRunningBelowTarget(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	start := now.Add(-30 * time.Minute) // 14:00 today
	target := 8 * time.Hour
	in := domain.StatusInputs{
		Now: now,
		Day: domain.Day{
			Logged: 4 * time.Hour, Active: &start, Target: target,
		},
		Target:       target,
		Palette:      pal(),
		LookupDayOff: noLookup,
	}
	got := domain.BuildStatusSegment(in)
	// active.Add(target - logged) = 14:00 + (8h - 4h) = 18:00.
	if !strings.Contains(got, "→18:00") {
		t.Errorf("ETA should be 18:00: %q", got)
	}
}

func TestBuildStatusSegment_DayOffBanner(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	dayOff := &domain.DayOff{Date: now, Kind: domain.KindHoliday, Label: "Tag der Arbeit"}
	in := domain.StatusInputs{
		Now: now, Day: domain.Day{Target: 0}, Target: 0,
		DayOff:       dayOff,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, "★ Tag der Arbeit") {
		t.Errorf("dayoff banner missing: %q", got)
	}
}

func TestBuildStatusSegment_StreakRendersAt3(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	target := 8 * time.Hour

	t.Run("streak < 3 not shown", func(t *testing.T) {
		in := domain.StatusInputs{
			Now: now,
			Day: domain.Day{Logged: target, Target: target}, Target: target,
			Streak: 2, Palette: pal(), LookupDayOff: noLookup,
		}
		if strings.Contains(domain.BuildStatusSegment(in), "Streak") {
			t.Error("streak 2 should not show")
		}
	})

	t.Run("streak >= 3 shown", func(t *testing.T) {
		in := domain.StatusInputs{
			Now: now,
			Day: domain.Day{Logged: target, Target: target}, Target: target,
			Streak: 5, Palette: pal(), LookupDayOff: noLookup,
		}
		if !strings.Contains(domain.BuildStatusSegment(in), "Streak 5") {
			t.Error("streak 5 should render")
		}
	})
}

func TestBuildStatusSegment_BurndownArrows(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	target := 8 * time.Hour

	tests := []struct {
		name string
		rep  domain.MonthBurndownReport
		want string
	}{
		{"on track over 1h", domain.MonthBurndownReport{Target: 160 * time.Hour, Saldo: 5 * time.Hour}, "▲ +5h"},
		{"under 1h shows nothing", domain.MonthBurndownReport{Target: 160 * time.Hour, Saldo: 30 * time.Minute}, ""},
		{"under -1h", domain.MonthBurndownReport{Target: 160 * time.Hour, Saldo: -3 * time.Hour}, "▼ -3h"},
		{"target=0 shows nothing", domain.MonthBurndownReport{Target: 0, Saldo: 5 * time.Hour}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := domain.StatusInputs{
				Now: now,
				Day: domain.Day{Logged: target, Target: target}, Target: target,
				Burndown:     tc.rep,
				Palette:      pal(),
				LookupDayOff: noLookup,
			}
			got := domain.BuildStatusSegment(in)
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

func TestBuildStatusSegment_DayOffOnlyStillRenders(t *testing.T) {
	// total=0, dots="", but DayOff != nil → segment should render.
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	dayOff := &domain.DayOff{Date: now, Kind: domain.KindVacation, Label: "Brückentag"}
	in := domain.StatusInputs{
		Now: now, Day: domain.Day{}, Target: 0,
		DayOff:       dayOff,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if got == "" {
		t.Error("dayoff alone should produce a segment")
	}
	if !strings.Contains(got, "Brückentag") {
		t.Errorf("dayoff banner missing label: %q", got)
	}
}

// Active session before midnight (negative elapsed) clamps to 0.
func TestBuildStatusSegment_NegativeElapsedClampsToZero(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	start := now.Add(time.Hour) // 15:30 — in the future relative to now
	target := 8 * time.Hour
	in := domain.StatusInputs{
		Now:    now,
		Day:    domain.Day{Active: &start, Target: target},
		Target: target, Palette: pal(), LookupDayOff: noLookup,
	}
	got := domain.BuildStatusSegment(in)
	// Live counter should render 0:00 — neither negative nor crashed.
	if !strings.Contains(got, "▶ 0:00") {
		t.Errorf("expected ▶ 0:00 for clamped elapsed, got: %q", got)
	}
}

// — BuildPaceDots —

func TestBuildPaceDots_EmptyWeekReturnsEmpty(t *testing.T) {
	if got := domain.BuildPaceDots(nil, time.Now(), noLookup, pal()); got != "" {
		t.Errorf("empty week should yield empty dots, got %q", got)
	}
}

func TestBuildPaceDots_HitGreenMissedDimRunningYellow(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local) // Wed
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	tue := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	wed := time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local)
	start := now.Add(-time.Hour)
	week := []domain.WeekDay{
		{Date: mon, Target: 8 * time.Hour, Logged: 8 * time.Hour},
		{Date: tue, Target: 8 * time.Hour, Logged: 4 * time.Hour},
		{Date: wed, Target: 8 * time.Hour, Logged: time.Hour, IsToday: true, Active: &start},
	}
	got := domain.BuildPaceDots(week, now, noLookup, pal())
	for _, want := range []string{
		pal().Green + "]●",  // Mon hit
		pal().Dim + "]○",    // Tue miss
		pal().Yellow + "]●", // Wed running below target
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildPaceDots_DayOffGlyphPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	fri := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{{Date: fri, Target: 8 * time.Hour}}

	tests := []struct {
		kind  domain.Kind
		glyph string
	}{
		{domain.KindHoliday, "★"},
		{domain.KindVacation, "☼"},
		{domain.KindSick, "✚"},
		{domain.Kind("unknown"), "○"},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			lookup := func(d time.Time) (domain.DayOff, bool) {
				if d.Equal(fri) {
					return domain.DayOff{Date: d, Kind: tc.kind, Label: "T"}, true
				}
				return domain.DayOff{}, false
			}
			got := domain.BuildPaceDots(week, now, lookup, pal())
			want := pal().Cyan + "]" + tc.glyph
			if !strings.Contains(got, want) {
				t.Errorf("kind %q expected %q in %q", tc.kind, want, got)
			}
		})
	}
}

func TestBuildStatusSegment_DayOffBannerGlyphPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	tests := []struct {
		kind  domain.Kind
		glyph string
	}{
		{domain.KindHoliday, "★"},
		{domain.KindVacation, "☼"},
		{domain.KindSick, "✚"},
		{domain.Kind("unknown"), "·"},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			d := &domain.DayOff{Date: now, Kind: tc.kind, Label: "Test"}
			in := domain.StatusInputs{
				Now: now, Day: domain.Day{}, Target: 0,
				DayOff:       d,
				LookupDayOff: noLookup,
				Palette:      pal(),
			}
			got := domain.BuildStatusSegment(in)
			if !strings.Contains(got, tc.glyph+" Test") {
				t.Errorf("kind %q expected glyph %q in banner: %q", tc.kind, tc.glyph, got)
			}
		})
	}
}

func TestBuildStatusSegment_DotsAttached(t *testing.T) {
	// Ensure non-empty dots reach the segment string.
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	wed := time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local)
	target := 8 * time.Hour
	week := []domain.WeekDay{
		{Date: mon, Target: target, Logged: target}, // hit
		{Date: wed, Target: target, Logged: 0, IsToday: true},
	}
	in := domain.StatusInputs{
		Now:  now,
		Day:  domain.Day{Logged: 30 * time.Minute, Target: target},
		Week: week, Target: target,
		Palette:      pal(),
		LookupDayOff: noLookup,
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, "●") {
		t.Errorf("dots should be attached, got %q", got)
	}
}

func TestBuildPaceDots_WeekendsSkipped(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local) // Sat
	sat := time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local)
	sun := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{
		{Date: sat, Target: 8 * time.Hour, Logged: 4 * time.Hour},
		{Date: sun, Target: 8 * time.Hour, Logged: 4 * time.Hour},
	}
	got := domain.BuildPaceDots(week, now, noLookup, pal())
	if got != "" {
		t.Errorf("Sat+Sun only week should yield empty, got %q", got)
	}
}

func TestBuildPaceDots_NilLookupTreatedAsNoDayOffs(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{{Date: mon, Target: 8 * time.Hour, Logged: 8 * time.Hour}}
	got := domain.BuildPaceDots(week, now, nil, pal())
	if !strings.Contains(got, "●") {
		t.Errorf("nil lookup should still render hit dot, got %q", got)
	}
}

func TestBuildPaceDots_AllMissedReturnsEmpty(t *testing.T) {
	// any flag stays false when no hit/today-running/dayoff dot lands. The
	// row is suppressed even though loop iterations created ○ dots.
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	tue := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{
		{Date: mon, Target: 8 * time.Hour, Logged: time.Hour},
		{Date: tue, Target: 8 * time.Hour, Logged: time.Hour},
	}
	if got := domain.BuildPaceDots(week, now, noLookup, pal()); got != "" {
		t.Errorf("all-missed week should yield empty, got %q", got)
	}
}
