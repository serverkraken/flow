package timefmt

import "time"

// StaffelWords sind die übersetzbaren Wörter der Datumsstaffel. timefmt kennt
// kein i18n — die Wörter kommen von außen herein, damit dieses Paket
// sprachfrei bleibt und trotzdem beide Kataloge bedient (Katalog 3.12: die
// Wörter der Datumsstaffel werden übersetzt, Inhalte nie).
type StaffelWords struct {
	Today    string    // "heute" / "today"
	Weekdays [7]string // Index = time.Weekday: Sonntag … Samstag
}

// Staffel schreibt einen Zeitpunkt in der gestaffelten Form aus Katalog 3.10.
// Vier Sprossen, nie ausgeschrieben:
//
//	heute 14:32   weniger als 24 h
//	Fr            zwei bis sieben Tage
//	11.08.        dieses Jahr
//	08.11.25      älter
//
// Die Ausgabe gehört immer in die rechte Spalte, immer in Monospace — damit
// das Auge eine Spalte abtastet statt zu suchen.
//
// Die Nullzeit ergibt den leeren String: "kein Datum" ist eine Aussage, die
// eine Liste nicht mit einer Zahl übertünchen darf.
func Staffel(at, now time.Time, w StaffelWords) string {
	if at.IsZero() {
		return ""
	}
	at = at.Local()
	now = now.Local()

	diff := now.Sub(at)
	if diff < 0 {
		// Ein Zeitpunkt in der Zukunft (Uhrendrift, importierte Daten) zählt
		// als jetzt — lieber "heute" als eine negative Altersangabe.
		diff = 0
	}
	if diff < 24*time.Hour {
		return w.Today + " " + at.Format("15:04")
	}
	if diff < 8*24*time.Hour {
		// Der Wochentag ist innerhalb einer Woche eindeutig; ab dem achten Tag
		// wäre er es nicht mehr und die Sprosse fällt aufs Datum.
		return w.Weekdays[int(at.Weekday())]
	}
	if at.Year() == now.Year() {
		return at.Format("02.01.")
	}
	return at.Format("02.01.06")
}
