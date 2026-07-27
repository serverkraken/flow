package statusline

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func tpal() StatusPalette { return DefaultStatusPalette() }

func TestBuildPaceDots_EmptyWeekReturnsEmpty(t *testing.T) {
	if got := buildPaceDots(nil, false, tpal()); got != "" {
		t.Errorf("empty week should yield empty dots, got %q", got)
	}
}

func TestBuildPaceDots_HitGreenMissedDimRunningCyan(t *testing.T) {
	week := []WeekDay{
		{Weekday: time.Monday, TargetMin: 480, LoggedMin: 480},                  // hit
		{Weekday: time.Tuesday, TargetMin: 480, LoggedMin: 240},                 // miss
		{Weekday: time.Wednesday, TargetMin: 480, LoggedMin: 60, IsToday: true}, // running below target
	}
	got := buildPaceDots(week, true, tpal())
	for _, want := range []string{
		tpal().Green + "]●", // Mon hit
		tpal().Dim + "]○",   // Tue miss
		tpal().Cyan + "]●",  // Wed running below target
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildPaceDots_DayOffGlyphPerKind(t *testing.T) {
	p := tpal()
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
			week := []WeekDay{{Weekday: time.Friday, TargetMin: 480, DayOffKind: tc.kind}}
			got := buildPaceDots(week, false, p)
			want := tc.color + "]●"
			if !strings.Contains(got, want) {
				t.Errorf("kind %q expected %q in %q", tc.kind, want, got)
			}
		})
	}
}

func TestBuildPaceDots_WeekendsSkipped(t *testing.T) {
	week := []WeekDay{
		{Weekday: time.Saturday, TargetMin: 480, LoggedMin: 240},
		{Weekday: time.Sunday, TargetMin: 480, LoggedMin: 240},
	}
	if got := buildPaceDots(week, false, tpal()); got != "" {
		t.Errorf("Sat+Sun only week should yield empty, got %q", got)
	}
}

func TestBuildPaceDots_AllMissedReturnsEmpty(t *testing.T) {
	week := []WeekDay{
		{Weekday: time.Monday, TargetMin: 480, LoggedMin: 60},
		{Weekday: time.Tuesday, TargetMin: 480, LoggedMin: 60},
	}
	if got := buildPaceDots(week, false, tpal()); got != "" {
		t.Errorf("all-missed week should yield empty, got %q", got)
	}
}

// A non-today running flag must not turn an empty today into a running dot —
// classify only treats today+running as paceRunning.
func TestClassify_RunningOnlyForToday(t *testing.T) {
	// today, running → paceRunning
	if k := classify(WeekDay{Weekday: time.Wednesday, TargetMin: 480, LoggedMin: 0, IsToday: true}, true); k != paceRunning {
		t.Errorf("today+running should be paceRunning, got %v", k)
	}
	// today, not running → paceMissed
	if k := classify(WeekDay{Weekday: time.Wednesday, TargetMin: 480, LoggedMin: 0, IsToday: true}, false); k != paceMissed {
		t.Errorf("today+idle should be paceMissed, got %v", k)
	}
	// not today but running flag on → paceMissed
	if k := classify(WeekDay{Weekday: time.Monday, TargetMin: 480, LoggedMin: 0, IsToday: false}, true); k != paceMissed {
		t.Errorf("non-today should never be paceRunning, got %v", k)
	}
}

func TestKindStatusColor_PerKind(t *testing.T) {
	p := tpal()
	tests := []struct {
		kind domain.Kind
		want string
	}{
		{domain.KindHoliday, p.Blue},
		{domain.KindVacation, p.Purple},
		{domain.KindSick, p.Orange},
		{domain.Kind("unknown"), p.Dim},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := KindStatusColor(tc.kind, p); got != tc.want {
				t.Errorf("KindStatusColor(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}
