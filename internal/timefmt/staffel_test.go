package timefmt_test

// Die Datumsstaffel (Elementkatalog 3.10). Vier Sprossen, nie ausgeschrieben:
//
//	heute 14:32   weniger als 24 h   — Uhrzeit dazu, weil es heute noch zählt
//	Fr            zwei bis sieben Tage — Wochentag allein, kürzer als jedes Datum
//	11.08.        dieses Jahr         — Tag und Monat, ohne Jahr
//	08.11.25      älter               — Jahr zweistellig hinten dran

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/timefmt"
)

func words() timefmt.StaffelWords {
	return timefmt.StaffelWords{
		Today:    "heute",
		Weekdays: [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
	}
}

func TestStaffel(t *testing.T) {
	// Donnerstag, 20.08.2026, 16:00 — in LOKALER Zeit aufgebaut, weil die
	// Staffel lokal ausgibt (die Zeitpunkte liegen als UTC in der Datenbank,
	// gelesen wird in der Zone des Betrachters). So hängt der Test nicht an
	// der Zone der Maschine, auf der er läuft.
	now := time.Date(2026, 8, 20, 16, 0, 0, 0, time.Local)

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"vor Minuten", now.Add(-20 * time.Minute), "heute 15:40"},
		{"vor Stunden", now.Add(-5 * time.Hour), "heute 11:00"},
		{"knapp unter 24 h", now.Add(-23 * time.Hour), "heute 17:00"},
		{"gestern, über 24 h", now.Add(-30 * time.Hour), "Mi"},
		{"vor drei Tagen", now.AddDate(0, 0, -3), "Mo"},
		{"vor sieben Tagen", now.AddDate(0, 0, -7), "Do"},
		{"dieses Jahr", time.Date(2026, 8, 11, 9, 0, 0, 0, time.Local), "11.08."},
		{"Jahreswechsel zurück", time.Date(2025, 11, 8, 9, 0, 0, 0, time.Local), "08.11.25"},
		{"Zukunft zählt als jetzt", now.Add(2 * time.Hour), "heute 18:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := timefmt.Staffel(c.at, now, words()); got != c.want {
				t.Errorf("Staffel(%s) = %q, erwartet %q", c.at.Format(time.RFC3339), got, c.want)
			}
		})
	}
}

// Die Nullzeit ist kein Datum — eine Liste darf dafür nichts behaupten.
func TestStaffel_ZeroTimeIsEmpty(t *testing.T) {
	now := time.Date(2026, 8, 20, 16, 0, 0, 0, time.Local)
	if got := timefmt.Staffel(time.Time{}, now, words()); got != "" {
		t.Errorf("Nullzeit muss leer bleiben, got %q", got)
	}
}

// Der achte Tag fällt auf die Datumssprosse — sonst hieße "Do" mal heute vor
// einer Woche und mal vor zwei.
func TestStaffel_EighthDayFallsToTheDateRung(t *testing.T) {
	now := time.Date(2026, 8, 20, 16, 0, 0, 0, time.Local)
	if got := timefmt.Staffel(now.AddDate(0, 0, -8), now, words()); got != "12.08." {
		t.Errorf("achter Tag = %q, erwartet 12.08.", got)
	}
}
