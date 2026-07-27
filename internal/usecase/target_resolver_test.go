package usecase_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestTargetResolver_Priority(t *testing.T) {
	loc := time.UTC
	fri := time.Date(2026, 6, 19, 0, 0, 0, 0, loc)    // Friday
	sat := time.Date(2026, 6, 20, 0, 0, 0, 0, loc)    // Saturday
	vacDay := time.Date(2026, 6, 18, 0, 0, 0, 0, loc) // Thursday, vacation half-day

	r := usecase.TargetResolver{
		Default: 8 * time.Hour,
		Weekday: [7]*time.Duration{
			time.Friday:   durPtr(6 * time.Hour),
			time.Saturday: durPtr(0), // explicit "no work Saturday"
		},
		DayOffs: map[string]domain.DayOff{
			vacDay.Format("2006-01-02"): {Date: vacDay, Kind: domain.KindVacation, Target: 4 * time.Hour},
		},
	}

	if got := r.For(fri); got != 6*time.Hour {
		t.Errorf("friday override: got %v want 6h", got)
	}
	if got := r.For(sat); got != 0 {
		t.Errorf("saturday explicit-0: got %v want 0", got)
	}
	if got := r.For(vacDay); got != 4*time.Hour {
		t.Errorf("dayoff override wins: got %v want 4h", got)
	}
	if !r.IsDayOff(vacDay) {
		t.Errorf("vacDay should be a day-off")
	}
	if r.IsWorkday(sat) {
		t.Errorf("saturday is weekend, not a workday")
	}
	if !r.IsWorkday(fri) {
		t.Errorf("friday should be a workday")
	}
	if r.IsWorkday(vacDay) {
		t.Errorf("vacation day is not a workday")
	}
}

func durPtr(d time.Duration) *time.Duration { return &d }
