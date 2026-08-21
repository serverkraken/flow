package domain

import "time"

// MonthLedger ist ein Monat in der Jahressicht der Historie (Screen 32):
// erfasst, Soll, und ob der Monat noch läuft.
type MonthLedger struct {
	Month   time.Time // der Erste des Monats, lokal
	Logged  time.Duration
	Target  time.Duration // Σ Tagesziel über alle Tage des Monats (Frei-Tage zählen 0)
	Current bool          // der Monat, in dem now liegt
	Future  bool          // liegt ganz nach now
}

// Saldo ist erfasst minus Soll.
func (m MonthLedger) Saldo() time.Duration { return m.Logged - m.Target }
