package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestPaceDotFor(t *testing.T) {
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.Local) // Tue
	today := time.Date(2026, 5, 12, 0, 0, 0, 0, time.Local)

	target8h := 8 * time.Hour
	startNow := now.Add(-30 * time.Minute)

	cases := []struct {
		name   string
		day    domain.WeekDay
		dayOff *domain.DayOff
		want   domain.PaceDotKind
	}{
		{
			name: "day-off wins over everything",
			day:  domain.WeekDay{Date: today, Target: target8h, IsToday: true},
			dayOff: &domain.DayOff{
				Date: today, Kind: domain.KindVacation,
			},
			want: domain.PaceDotDayOff,
		},
		{
			name: "logged ≥ target → Hit",
			day: domain.WeekDay{
				Date: today, Target: target8h, Logged: 9 * time.Hour,
			},
			want: domain.PaceDotHit,
		},
		{
			name: "today running, no target hit yet → Running",
			day: domain.WeekDay{
				Date: today, Target: target8h, Logged: time.Hour,
				IsToday: true, Active: &startNow,
			},
			want: domain.PaceDotRunning,
		},
		{
			name: "workday without activity → Missed",
			day:  domain.WeekDay{Date: today, Target: target8h},
			want: domain.PaceDotMissed,
		},
		{
			name: "target=0 (free workday by config) → Missed (not Hit)",
			day:  domain.WeekDay{Date: today, Target: 0, Logged: time.Hour},
			want: domain.PaceDotMissed,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.PaceDotFor(tt.day, now, tt.dayOff)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPaceDotGlyph(t *testing.T) {
	cases := []struct {
		k    domain.PaceDotKind
		want string
	}{
		{domain.PaceDotMissed, "○"},
		{domain.PaceDotHit, "●"},
		{domain.PaceDotRunning, "●"},
		{domain.PaceDotDayOff, "●"},
	}
	for _, tt := range cases {
		if got := domain.PaceDotGlyph(tt.k); got != tt.want {
			t.Errorf("kind %v: got %q, want %q", tt.k, got, tt.want)
		}
	}
}
