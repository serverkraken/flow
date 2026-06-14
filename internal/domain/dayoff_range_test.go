package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestExpandRange_SkipsWeekends(t *testing.T) {
	// Mon 2026-06-15 .. Sun 2026-06-21, skip weekends → Mon..Fri = 5 days.
	got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 21), domain.KindVacation, "Sommer", 0, true)
	if len(got) != 5 {
		t.Fatalf("want 5 weekday entries, got %d", len(got))
	}
	for _, o := range got {
		if o.Kind != domain.KindVacation || o.Label != "Sommer" || o.Target != 0 {
			t.Fatalf("unexpected entry: %+v", o)
		}
		if wd := o.Date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("weekend leaked in: %s", o.Date)
		}
	}
}

func TestExpandRange_IncludesWeekendsAndHalfDay(t *testing.T) {
	got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 21), domain.KindSick, "", 4*time.Hour, false)
	if len(got) != 7 {
		t.Fatalf("want 7 entries incl. weekend, got %d", len(got))
	}
	if got[0].Target != 4*time.Hour {
		t.Fatalf("want half-day target carried, got %v", got[0].Target)
	}
}

func TestExpandRange_SingleDayAndReversedRange(t *testing.T) {
	if got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 15), domain.KindVacation, "", 0, false); len(got) != 1 {
		t.Fatalf("single day: want 1, got %d", len(got))
	}
	if got := domain.ExpandRange(d(2026, 6, 21), d(2026, 6, 15), domain.KindVacation, "", 0, false); got != nil {
		t.Fatalf("reversed range: want nil, got %d", len(got))
	}
}
