package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestGermanHolidays_BundesweitAlwaysIncluded(t *testing.T) {
	hs := domain.GermanHolidays(2026, "DE", time.Local)
	must := map[string]bool{
		"2026-01-01": false, // Neujahr
		"2026-05-01": false, // Tag der Arbeit
		"2026-10-03": false, // Tag der Deutschen Einheit
		"2026-12-25": false, // 1. Weihnachtstag
		"2026-12-26": false, // 2. Weihnachtstag
	}
	for _, h := range hs {
		if _, ok := must[h.Date.Format("2006-01-02")]; ok {
			must[h.Date.Format("2006-01-02")] = true
		}
	}
	for k, seen := range must {
		if !seen {
			t.Errorf("missing bundesweit holiday: %s", k)
		}
	}
}

func TestGermanHolidays_NRWFeatures(t *testing.T) {
	hs := domain.GermanHolidays(2026, "NW", time.Local)
	// Easter 2026 is 2026-04-05.
	want := map[string]bool{
		"2026-04-03": false, // Karfreitag
		"2026-04-06": false, // Ostermontag
		"2026-05-14": false, // Christi Himmelfahrt (Easter + 39)
		"2026-05-25": false, // Pfingstmontag (Easter + 50)
		"2026-06-04": false, // Fronleichnam (Easter + 60)
		"2026-11-01": false, // Allerheiligen
	}
	for _, h := range hs {
		if _, ok := want[h.Date.Format("2006-01-02")]; ok {
			want[h.Date.Format("2006-01-02")] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("NRW missing holiday: %s", k)
		}
	}
}

func TestGermanHolidays_BavariaHasFronleichnam(t *testing.T) {
	hs := domain.GermanHolidays(2026, "BY", time.Local)
	for _, h := range hs {
		if h.Label == "Fronleichnam" {
			return
		}
	}
	t.Error("Bayern should include Fronleichnam")
}

func TestGermanHolidays_BerlinNoFronleichnam(t *testing.T) {
	hs := domain.GermanHolidays(2026, "BE", time.Local)
	for _, h := range hs {
		if h.Label == "Fronleichnam" {
			t.Error("Berlin should NOT include Fronleichnam")
		}
	}
}

func TestGermanHolidays_DEExcludesLandSpecific(t *testing.T) {
	// "DE" is the bundesweit-only filter — region-specific holidays must NOT
	// appear (e.g. Allerheiligen, Heilige Drei Könige, Frauentag).
	hs := domain.GermanHolidays(2026, "DE", time.Local)
	forbidden := map[string]bool{
		"Heilige Drei Könige":       true,
		"Internationaler Frauentag": true,
		"Fronleichnam":              true,
		"Mariä Himmelfahrt":         true,
		"Weltkindertag":             true,
		"Reformationstag":           true,
		"Allerheiligen":             true,
		"Buß- und Bettag":           true,
	}
	for _, h := range hs {
		if forbidden[h.Label] {
			t.Errorf("DE should not include %q", h.Label)
		}
	}
}

func TestNormalizeLand(t *testing.T) {
	// normalizeLand is unexported; exercise it through GermanHolidays
	// where the alias resolution is observable in the result count.
	tests := []struct {
		in     string
		sample string // a holiday label that should appear in the result set
	}{
		{"NRW", "Fronleichnam"},
		{"nw", "Fronleichnam"},
		{" NW ", "Fronleichnam"},
		{"Bayern", "Fronleichnam"},
		{"BAYERN", "Fronleichnam"},
		{"baden-württemberg", "Fronleichnam"},
		{"BAWÜ", "Fronleichnam"},
		{"BAWUE", "Fronleichnam"},
		{"baden-wuerttemberg", "Fronleichnam"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			hs := domain.GermanHolidays(2026, tc.in, time.Local)
			found := false
			for _, h := range hs {
				if h.Label == tc.sample {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%q did not yield %q (alias resolution failed)", tc.in, tc.sample)
			}
		})
	}
}

func TestGermanHolidays_EmptyLandFallsBackToDE(t *testing.T) {
	// Empty land normalizes to "DE" — confirm by checking that no
	// region-specific holiday (e.g. Fronleichnam) appears.
	hs := domain.GermanHolidays(2026, "", time.Local)
	for _, h := range hs {
		if h.Label == "Fronleichnam" {
			t.Error("empty land should yield bundesweit-only set, not Fronleichnam")
		}
	}
	// Sanity: bundesweit set is non-empty.
	if len(hs) == 0 {
		t.Error("empty land yielded no holidays at all")
	}
}

func TestGermanHolidays_BusBettagOnlyInSaxony(t *testing.T) {
	for _, land := range []string{"NW", "BY", "DE", "BE"} {
		hs := domain.GermanHolidays(2026, land, time.Local)
		for _, h := range hs {
			if h.Label == "Buß- und Bettag" {
				t.Errorf("Buß- und Bettag should not appear in %s", land)
			}
		}
	}
	hs := domain.GermanHolidays(2026, "SN", time.Local)
	found := false
	for _, h := range hs {
		if h.Label == "Buß- und Bettag" {
			// Always between Nov 16 and Nov 22, on a Wednesday.
			if h.Date.Weekday() != time.Wednesday {
				t.Errorf("Buß- und Bettag is not Wednesday: %v", h.Date)
			}
			if h.Date.Day() < 16 || h.Date.Day() > 22 {
				t.Errorf("Buß- und Bettag day out of range: %d", h.Date.Day())
			}
			found = true
		}
	}
	if !found {
		t.Error("Buß- und Bettag missing from Sachsen list")
	}
}

func TestGermanHolidays_AllKindAreHoliday(t *testing.T) {
	for _, h := range domain.GermanHolidays(2026, "NW", time.Local) {
		if h.Kind != domain.KindHoliday {
			t.Errorf("%q has kind %q, want holiday", h.Label, h.Kind)
		}
		if h.Target != 0 {
			t.Errorf("%q has Target %v, want 0 (full day off)", h.Label, h.Target)
		}
	}
}

func TestEasterSunday_KnownDates(t *testing.T) {
	// easterSunday is unexported; exercise via Karfreitag (Easter - 2).
	tests := map[int]string{
		2024: "2024-03-29", // Easter 03-31
		2025: "2025-04-18", // Easter 04-20
		2026: "2026-04-03", // Easter 04-05
		2027: "2027-03-26", // Easter 03-28
	}
	for year, wantKar := range tests {
		hs := domain.GermanHolidays(year, "DE", time.Local)
		var got string
		for _, h := range hs {
			if h.Label == "Karfreitag" {
				got = h.Date.Format("2006-01-02")
				break
			}
		}
		if got != wantKar {
			t.Errorf("year %d: Karfreitag = %s, want %s", year, got, wantKar)
		}
	}
}
