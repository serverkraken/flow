package worktime_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestGermanHolidays_BundesweitAlwaysIncluded(t *testing.T) {
	hs := worktime.GermanHolidays(2026, "DE")
	must := map[string]bool{
		"2026-01-01": false, // Neujahr
		"2026-05-01": false, // Tag der Arbeit
		"2026-10-03": false, // Tag der Deutschen Einheit
		"2026-12-25": false, // 1. Weihnachtstag
		"2026-12-26": false, // 2. Weihnachtstag
	}
	for _, h := range hs {
		key := h.Date.Format("2006-01-02")
		if _, ok := must[key]; ok {
			must[key] = true
		}
	}
	for k, ok := range must {
		if !ok {
			t.Errorf("missing bundesweit holiday: %s", k)
		}
	}
}

func TestGermanHolidays_NRWFeatures(t *testing.T) {
	hs := worktime.GermanHolidays(2026, "NW")
	want := map[string]bool{
		"2026-11-01": false, // Allerheiligen
		"2026-04-03": false, // Karfreitag (Easter is 2026-04-05, so Friday is 04-03)
		"2026-04-06": false, // Ostermontag
		"2026-05-14": false, // Christi Himmelfahrt (39 days after Easter)
		"2026-05-25": false, // Pfingstmontag (50 days after Easter)
		"2026-06-04": false, // Fronleichnam (60 days after Easter)
	}
	for _, h := range hs {
		key := h.Date.Format("2006-01-02")
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("NRW: missing holiday %s", k)
		}
	}
}

func TestGermanHolidays_NRWAlias(t *testing.T) {
	a := worktime.GermanHolidays(2026, "NRW")
	b := worktime.GermanHolidays(2026, "NW")
	if len(a) != len(b) {
		t.Errorf("NRW alias mismatch: NRW=%d, NW=%d", len(a), len(b))
	}
}

func TestGermanHolidays_BavariaHasFronleichnam(t *testing.T) {
	hs := worktime.GermanHolidays(2026, "BY")
	for _, h := range hs {
		if h.Label == "Fronleichnam" {
			return
		}
	}
	t.Error("Bayern should include Fronleichnam")
}

func TestGermanHolidays_BerlinNoFronleichnam(t *testing.T) {
	hs := worktime.GermanHolidays(2026, "BE")
	for _, h := range hs {
		if h.Label == "Fronleichnam" {
			t.Error("Berlin should NOT include Fronleichnam")
		}
	}
}

func TestSyncGermanHolidays_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	worktime.ResetCachesForTesting()

	a1, s1, err := worktime.SyncGermanHolidays(2026, "NW")
	if err != nil {
		t.Fatal(err)
	}
	if a1 == 0 {
		t.Error("first sync added 0 entries")
	}
	worktime.ResetCachesForTesting()

	a2, s2, err := worktime.SyncGermanHolidays(2026, "NW")
	if err != nil {
		t.Fatal(err)
	}
	if a2 != 0 {
		t.Errorf("second sync should be no-op, added %d", a2)
	}
	if s2 == 0 {
		t.Error("second sync should report skipped")
	}
	_ = s1
}

func TestSyncGermanHolidays_PreservesUserEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	worktime.ResetCachesForTesting()

	// User adds a vacation on Karfreitag — the sync should NOT clobber it.
	karfreitag := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)
	if err := worktime.AddDayOff(karfreitag, worktime.KindVacation, "Selber genommen"); err != nil {
		t.Fatal(err)
	}
	worktime.ResetCachesForTesting()

	if _, _, err := worktime.SyncGermanHolidays(2026, "NW"); err != nil {
		t.Fatal(err)
	}
	worktime.ResetCachesForTesting()

	got, ok := worktime.LookupDayOff(karfreitag)
	if !ok {
		t.Fatal("entry vanished")
	}
	if got.Kind != worktime.KindVacation {
		t.Errorf("kind got %v, want vacation (sync overwrote user intent)", got.Kind)
	}
}
