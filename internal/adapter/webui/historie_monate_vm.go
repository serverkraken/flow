package webui

import (
	"context"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Historie „Monat für Monat" (Screen 32): das Jahr als Tabelle — erfasst,
// Soll, Differenz, Karten je Monat, die Summe darunter. Eine Zeile führt in
// den Monat des Kalenders.

// MonateRow ist ein Monat.
type MonateRow struct {
	Label    string // "August"
	Logged   string // "68:20"
	Target   string // "144:00"
	Diff     string // "−6:45" / "+4:40" / "—" (läuft oder Zukunft)
	DiffTone string // "text-live" | "text-red" | "text-faint"
	Cards    int
	Href     string // Kalender-Monat
	Current  bool
	Future   bool
}

// HistorieMonateVM treibt Seite und Fragment.
type HistorieMonateVM struct {
	User        string
	Year        int
	YearStr     string
	PrevHref    string
	NextHref    string // "" im laufenden Jahr
	FragmentURL string
	Rows        []MonateRow
	TotalLogged string
	TotalTarget string
	TotalDiff   string
	TotalTone   string
	TotalCards  int
	Err         string
}

// BuildHistorieMonate ordnet die Monatsbilanz und zählt die Karten je
// Monat nach ihrem Anlagedatum. Zukünftige Monate stehen ohne Erfasstes
// da, der laufende ohne Differenz — eine Differenz im laufenden Monat
// liest sich als Rückstand, der keiner ist.
func BuildHistorieMonate(ctx context.Context, year int, months []domain.MonthLedger, docs []domain.Document, now time.Time) HistorieMonateVM {
	vm := HistorieMonateVM{Year: year, YearStr: strconv.Itoa(year)}
	vm.PrevHref = "/historie?view=monate&jahr=" + strconv.Itoa(year-1)
	vm.FragmentURL = "/ui/historie/monate?jahr=" + strconv.Itoa(year)
	if year < now.Year() {
		vm.NextHref = "/historie?view=monate&jahr=" + strconv.Itoa(year+1)
	}
	cards := make([]int, 13)
	for _, d := range docs {
		if d.CreatedAt.Year() == year {
			cards[d.CreatedAt.Month()]++
		}
	}
	var logged, target time.Duration
	for _, m := range months {
		row := MonateRow{
			Label:   monthText(ctx, m.Month.Month()),
			Cards:   cards[m.Month.Month()],
			Href:    "/historie?view=cal&cal=month&week=" + m.Month.Format("2006-01-02"),
			Current: m.Current,
			Future:  m.Future,
			Target:  fmtClock(m.Target),
		}
		if !m.Future {
			row.Logged = fmtClock(m.Logged)
			logged += m.Logged
			target += m.Target
		}
		if m.Future {
			row.Logged = "—"
		}
		row.Diff, row.DiffTone = monateDiff(m.Saldo(), m.Current || m.Future)
		vm.TotalCards += row.Cards
		vm.Rows = append(vm.Rows, row)
	}
	vm.TotalLogged = fmtClock(logged)
	vm.TotalTarget = fmtClock(target)
	vm.TotalDiff, vm.TotalTone = monateDiff(logged-target, false)
	return vm
}

// monateDiff schreibt die Differenz mit Vorzeichen — oder einen Strich,
// wenn sie noch nichts sagt.
func monateDiff(d time.Duration, open bool) (string, string) {
	if open {
		return "—", "text-faint"
	}
	switch {
	case d > 0:
		return "+" + fmtClock(d), "text-live"
	case d < 0:
		return "−" + fmtClock(-d), "text-red"
	}
	return "±0:00", "text-meta"
}
