package projects

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestAggregate covers all branches of the internal aggregate function:
// skipping wrong-project sessions, skipping running sessions, summing total/
// week/month durations, and computing earnings when a rate is set.
func TestAggregate(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) // Sunday 2026-06-21
	p := domain.Project{ID: "p1"}

	pid := "p1"
	otherPid := "p2"

	// Mon 2026-06-15 09:00–11:00 (2h, this week, this month)
	start1 := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop1 := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)

	// Mon 2026-05-12 09:00–10:00 (1h, prior month, prior week)
	start2 := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	// Session for wrong project — must be skipped.
	start3 := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	stop3 := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC)

	// Running session (no Stop) — must be skipped.
	start4 := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)

	sessions := []domain.WorkSession{
		{ID: "s1", ProjectID: &pid, Start: start1, Stop: &stop1},
		{ID: "s2", ProjectID: &pid, Start: start2, Stop: &stop2},
		{ID: "s3", ProjectID: &otherPid, Start: start3, Stop: &stop3}, // different project
		{ID: "s4", ProjectID: &pid, Start: start4},                    // running (no Stop)
	}

	agg := aggregate(p, sessions, now)

	// Total: s1 (2h) + s2 (1h) = 3h (running s4 excluded, wrong-proj s3 excluded)
	if agg.Total != 3*time.Hour {
		t.Errorf("Total = %v, want 3h", agg.Total)
	}

	// Week: only s1 (2026-06-15 is Mon of current week; now is 2026-06-21 Sun)
	if agg.Week != 2*time.Hour {
		t.Errorf("Week = %v, want 2h", agg.Week)
	}

	// Month: only s1 (s2 is prior month)
	if agg.Month != 2*time.Hour {
		t.Errorf("Month = %v, want 2h", agg.Month)
	}

	// Earnings: no rate set → empty string
	if agg.Earnings != "" {
		t.Errorf("Earnings without rate = %q, want \"\"", agg.Earnings)
	}

	// Set a rate and re-run to exercise the earnings branch.
	p.Rate = &domain.Money{Amount: 10000, Currency: "EUR"} // 10000 cents = 100€/h
	aggWithRate := aggregate(p, sessions, now)
	if aggWithRate.Earnings == "" {
		t.Error("Earnings with rate should be non-empty")
	}
}
