package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestDayOffWriter_Add_HappyPath(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	d := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	if err := w.Add(d, domain.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Lookup(d)
	if !ok {
		t.Fatal("entry not stored")
	}
	if got.Date.Hour() != 0 {
		t.Errorf("Date should be midnight, got %v", got.Date)
	}
	if got.Label != "Tag der Arbeit" {
		t.Errorf("Label = %q", got.Label)
	}
}

func TestDayOffWriter_Add_EmptyLabelDefaultsToKind(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	d := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if err := w.Add(d, domain.KindVacation, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Lookup(d)
	if got.Label != "Urlaub" {
		t.Errorf("Label = %q, want default 'Urlaub'", got.Label)
	}
}

func TestDayOffWriter_Add_InvalidKindFails(t *testing.T) {
	w := &usecase.DayOffWriter{Store: testutil.NewFakeDayOffStore()}
	err := w.Add(time.Now(), domain.Kind("nonsense"), "x")
	if err == nil {
		t.Error("expected error on invalid kind")
	}
}

func TestDayOffWriter_Add_StoreErrPropagates(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	store.Err = errors.New("boom")
	w := &usecase.DayOffWriter{Store: store}
	if err := w.Add(time.Now(), domain.KindHoliday, "x"); err == nil {
		t.Error("expected error")
	}
}

func TestDayOffWriter_AddRange_HappyPath(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	from := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local) // Mon..Fri = 5 days
	count, err := w.AddRange(from, to, domain.KindVacation, "Sommerurlaub")
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
	if len(store.Entries) != 5 {
		t.Errorf("stored = %d, want 5", len(store.Entries))
	}
}

func TestDayOffWriter_AddRange_InvertedRangeFails(t *testing.T) {
	w := &usecase.DayOffWriter{Store: testutil.NewFakeDayOffStore()}
	from := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local)
	if _, err := w.AddRange(from, to, domain.KindVacation, "x"); err == nil {
		t.Error("expected error for to < from")
	}
}

func TestDayOffWriter_AddRange_StopsOnError(t *testing.T) {
	// Custom store that fails on the 3rd Add.
	store := &countingDayOffStore{failAfter: 2}
	w := &usecase.DayOffWriter{Store: store}
	from := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	count, err := w.AddRange(from, to, domain.KindVacation, "x")
	if err == nil {
		t.Error("expected error after failAfter")
	}
	if count != 2 {
		t.Errorf("partial count = %d, want 2 successes before failure", count)
	}
}

func TestDayOffWriter_Remove(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	store := testutil.NewFakeDayOffStore(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "x"})
	w := &usecase.DayOffWriter{Store: store}
	if err := w.Remove(d); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup(d); ok {
		t.Error("entry should be removed")
	}
}

func TestDayOffWriter_SyncGermanHolidays_AddsAll(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	added, skipped, err := w.SyncGermanHolidays(2026, "NW", time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if added == 0 || skipped != 0 {
		t.Errorf("first sync: added=%d skipped=%d", added, skipped)
	}
}

func TestDayOffWriter_SyncGermanHolidays_Idempotent(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	if _, _, err := w.SyncGermanHolidays(2026, "NW", time.Local); err != nil {
		t.Fatal(err)
	}
	added, skipped, err := w.SyncGermanHolidays(2026, "NW", time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("re-sync added=%d, want 0", added)
	}
	if skipped == 0 {
		t.Error("re-sync should report skipped > 0")
	}
}

func TestDayOffWriter_SyncGermanHolidays_PreservesUserEntries(t *testing.T) {
	karfreitag := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)
	store := testutil.NewFakeDayOffStore(domain.DayOff{
		Date:  karfreitag,
		Kind:  domain.KindVacation,
		Label: "Eigene Wahl",
	})
	w := &usecase.DayOffWriter{Store: store}
	if _, _, err := w.SyncGermanHolidays(2026, "NW", time.Local); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Lookup(karfreitag)
	if got.Kind != domain.KindVacation {
		t.Errorf("user entry was overwritten: %+v", got)
	}
}

func TestDayOffWriter_SyncGermanHolidays_UnknownLandStillSyncsBundesweit(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	// Unknown land falls through normalizeLand and gets the bundesweit-only
	// set (since region-specific holidays don't apply).
	added, _, err := w.SyncGermanHolidays(2026, "XX", time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if added == 0 {
		t.Error("bundesweite Feiertage should still be added for unknown land")
	}
}

func TestDayOffWriter_SyncGermanHolidays_StoreAddErrPropagates(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	store.Err = errors.New("boom")
	w := &usecase.DayOffWriter{Store: store}
	if _, _, err := w.SyncGermanHolidays(2026, "NW", time.Local); err == nil {
		t.Error("expected error")
	}
}

// TestDayOffWriter_SyncGermanHolidays_RespectsLocation guards the
// injection contract that the function's doc comment promises. The
// previous hardcoded time.Local meant CI in a different $TZ saw
// holidays anchored at midnight-Berlin while the test compared against
// midnight-UTC — a silent date drift. Pass UTC explicitly and verify
// each stored date is at midnight-UTC.
func TestDayOffWriter_SyncGermanHolidays_RespectsLocation(t *testing.T) {
	store := testutil.NewFakeDayOffStore()
	w := &usecase.DayOffWriter{Store: store}
	added, _, err := w.SyncGermanHolidays(2026, "NW", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if added == 0 {
		t.Fatal("expected holidays to be added")
	}
	for _, entry := range store.List(time.Time{}, time.Time{}) {
		if entry.Date.Location() != time.UTC {
			t.Errorf("entry %s location = %s, want UTC", entry.Label, entry.Date.Location())
		}
		if h := entry.Date.Hour(); h != 0 {
			t.Errorf("entry %s hour = %d, want 0 (midnight UTC)", entry.Label, h)
		}
	}
}

// countingDayOffStore wraps the in-memory map but fails the (n+1)th Add.
// Used to test AddRange's partial-progress contract.
type countingDayOffStore struct {
	testutil.FakeDayOffStore
	failAfter int
	calls     int
}

func (s *countingDayOffStore) Add(off domain.DayOff) error {
	s.calls++
	if s.calls > s.failAfter {
		return errors.New("boom")
	}
	if s.Entries == nil {
		s.Entries = map[string]domain.DayOff{}
	}
	s.Entries[off.Date.Format("2006-01-02")] = off
	return nil
}
