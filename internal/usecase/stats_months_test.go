package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestStatsComputer_Months(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	stop1 := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC)
	sessions := []domain.WorkSession{
		{ID: "a", OwnerID: "u1", Start: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Stop: &stop1},
		{ID: "b", OwnerID: "u1", Start: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC), Stop: &stop2},
	}
	c := newComputer(sessions, set)
	months, err := c.Months(context.Background(), "u1", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 12 {
		t.Fatalf("12 Monate, got %d", len(months))
	}
	if months[5].Logged != 3*time.Hour || months[2].Logged != 2*time.Hour || months[0].Logged != 0 {
		t.Errorf("Logged: Juni=%v März=%v Jan=%v", months[5].Logged, months[2].Logged, months[0].Logged)
	}
	// Juni 2026: 22 Wochentage minus Fronleichnam (NRW) = 21 Werktage × 8 h.
	if months[5].Target != 168*time.Hour {
		t.Errorf("Soll Juni = %v, want 168h", months[5].Target)
	}
	if !months[5].Current || months[6].Current || !months[11].Future || months[4].Future {
		t.Errorf("Current/Future: %+v %+v", months[5], months[11])
	}
	if months[5].Saldo() != 3*time.Hour-168*time.Hour {
		t.Errorf("Saldo = %v", months[5].Saldo())
	}
}
