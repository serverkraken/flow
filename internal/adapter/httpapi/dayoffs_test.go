package httpapi_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/domain"
)

func TestDayOffs_AddAndList(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	target := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{
		Date:  target,
		Kind:  domain.KindVacation,
		Label: "Test Vacation",
	}

	if err := dayoffs.Add(off); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results := dayoffs.List(
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	)
	found := false
	for _, r := range results {
		if r.Date.Equal(target) && r.Kind == domain.KindVacation {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("added day-off not found in List results (got %d entries)", len(results))
	}
}

func TestDayOffs_Lookup(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	date := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{
		Date:  date,
		Kind:  domain.KindHoliday,
		Label: "Test Holiday",
	}

	if err := dayoffs.Add(off); err != nil {
		t.Fatalf("Add: %v", err)
	}

	found, ok := dayoffs.Lookup(date)
	if !ok {
		t.Fatal("Lookup returned not found")
	}
	if found.Kind != domain.KindHoliday {
		t.Errorf("kind = %q, want %q", found.Kind, domain.KindHoliday)
	}
}

func TestDayOffs_Remove(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	date := time.Date(2025, 4, 20, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{
		Date:  date,
		Kind:  domain.KindSick,
		Label: "Sick Day",
	}

	if err := dayoffs.Add(off); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify it's there
	_, ok := dayoffs.Lookup(date)
	if !ok {
		t.Fatal("day-off not found before Remove")
	}

	if err := dayoffs.Remove(date); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Should be gone now
	_, ok = dayoffs.Lookup(date)
	if ok {
		t.Error("day-off still found after Remove")
	}
}

func TestDayOffs_Remove_NonExistent_NoError(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	date := time.Date(2030, 7, 4, 0, 0, 0, 0, time.UTC)
	if err := dayoffs.Remove(date); err != nil {
		t.Errorf("Remove of non-existent day returned error: %v", err)
	}
}

func TestDayOffs_AddBatch(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	year := 2025
	offs := []domain.DayOff{
		{Date: time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC), Kind: domain.KindVacation, Label: "Vacation A"},
		{Date: time.Date(year, 6, 2, 0, 0, 0, 0, time.UTC), Kind: domain.KindVacation, Label: "Vacation B"},
	}

	if err := dayoffs.AddBatch(offs); err != nil {
		t.Fatalf("AddBatch: %v", err)
	}

	results := dayoffs.List(
		time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(year, 6, 2, 0, 0, 0, 0, time.UTC),
	)
	if len(results) < 2 {
		t.Errorf("expected at least 2 day-offs in batch result, got %d", len(results))
	}
}

func TestDayOffs_WithTarget(t *testing.T) {
	api := newTestAPI(t)
	dayoffs := httpapi.NewDayOffs(api.Client)

	date := time.Date(2025, 9, 12, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{
		Date:   date,
		Kind:   domain.KindVacation,
		Label:  "Half Day",
		Target: 4 * time.Hour,
	}

	if err := dayoffs.Add(off); err != nil {
		t.Fatalf("Add with target: %v", err)
	}

	found, ok := dayoffs.Lookup(date)
	if !ok {
		t.Fatal("Lookup returned not found")
	}
	if found.Target != 4*time.Hour {
		t.Errorf("target = %v, want 4h", found.Target)
	}
}
