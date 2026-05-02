package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkResolver(cfg domain.Config, cfgErr error, offs ...domain.DayOff) *usecase.TargetResolver {
	return &usecase.TargetResolver{
		Config:        &testutil.FakeConfigReader{Cfg: cfg, Err: cfgErr},
		DayOffs:       testutil.NewFakeDayOffStore(offs...),
		DefaultTarget: 8 * time.Hour,
	}
}

func TestTargetResolver_DayOffWins(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r := mkResolver(
		domain.Config{DefaultTarget: 8 * time.Hour},
		nil,
		domain.DayOff{Date: d, Kind: domain.KindHoliday, Target: 0}, // full off
	)
	if got := r.For(d); got != 0 {
		t.Errorf("dayoff Target=0 should win: got %v", got)
	}
}

func TestTargetResolver_DayOffHalfDay(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r := mkResolver(
		domain.Config{DefaultTarget: 8 * time.Hour},
		nil,
		domain.DayOff{Date: d, Kind: domain.KindVacation, Target: 4 * time.Hour},
	)
	if got := r.For(d); got != 4*time.Hour {
		t.Errorf("dayoff override should win: got %v", got)
	}
}

func TestTargetResolver_DayOffMinusOneFallsThrough(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r := mkResolver(
		domain.Config{DefaultTarget: 8 * time.Hour},
		nil,
		domain.DayOff{Date: d, Target: -1}, // forward-compat sentinel
	)
	if got := r.For(d); got != 8*time.Hour {
		t.Errorf("dayoff Target=-1 should fall through to config: got %v", got)
	}
}

func TestTargetResolver_PerWeekday(t *testing.T) {
	fri := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local) // Friday
	r := mkResolver(domain.Config{
		DefaultTarget: 8 * time.Hour,
		PerWeekday:    map[time.Weekday]time.Duration{time.Friday: 6 * time.Hour},
	}, nil)
	if got := r.For(fri); got != 6*time.Hour {
		t.Errorf("Friday override should win: got %v", got)
	}
}

func TestTargetResolver_DefaultFromConfig(t *testing.T) {
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local) // Monday, no override
	r := mkResolver(domain.Config{DefaultTarget: 8 * time.Hour}, nil)
	if got := r.For(mon); got != 8*time.Hour {
		t.Errorf("Monday no override: got %v want 8h", got)
	}
}

func TestTargetResolver_ConfigErrorFallsBackToFieldDefault(t *testing.T) {
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	r := mkResolver(domain.Config{}, errors.New("read failed"))
	if got := r.For(mon); got != 8*time.Hour {
		t.Errorf("config error should fall back to DefaultTarget: got %v", got)
	}
}

func TestTargetResolver_EmptyConfigUsesFieldDefault(t *testing.T) {
	// DefaultTarget=0 in config but resolver has 8h fallback.
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	r := mkResolver(domain.Config{}, nil)
	if got := r.For(mon); got != 8*time.Hour {
		t.Errorf("zero-config should fall through to resolver default: got %v", got)
	}
}

func TestTargetResolver_IsWorkday(t *testing.T) {
	r := mkResolver(domain.Config{DefaultTarget: 8 * time.Hour}, nil,
		domain.DayOff{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday})

	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{"Monday no dayoff", time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), true},
		{"Friday dayoff", time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), false},
		{"Saturday weekend", time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local), false},
		{"Sunday weekend", time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.IsWorkday(tc.date); got != tc.want {
				t.Errorf("IsWorkday(%v) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestTargetResolver_IsDayOff(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r := mkResolver(domain.Config{}, nil, domain.DayOff{Date: d})
	if !r.IsDayOff(d) {
		t.Error("configured dayoff should be reported")
	}
	if r.IsDayOff(time.Date(2026, 5, 5, 0, 0, 0, 0, time.Local)) {
		t.Error("non-configured date should not be dayoff")
	}
}
