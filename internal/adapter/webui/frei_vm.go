package webui

import (
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
)

// FreiRowVM.Hue feeds wocheDayOffTypeChip (woche_vm.go) for the row's
// .typechip/.tc-* tone — the same semantic day-off-kind→chip mapping Woche
// and Historie already share (Farb-Gesetz §7: fixed, semantic type colors,
// not a per-row hue wash). No local chip-color helper here anymore.

// FreiVM is the view model for the Frei page: a day-off capture form, the
// own-entries + holidays list for the year, and the settings card (ICS feed +
// Bundesland).
type FreiVM struct {
	User              string
	BundeslandCode    string // "NW" (drives the <select> selected option)
	BundeslandName    string // "Nordrhein-Westfalen" (list header)
	BundeslandOptions []FreiBundeslandOption
	Year              string
	IcsURL            string
	Rows              []FreiRowVM
	HasOwn            bool // at least one non-holiday entry (drives EmptyState)
	Err               string
}

// FreiBundeslandOption is one entry in the Bundesland <select>.
type FreiBundeslandOption struct{ Code, Name string }

// FreiRowVM is one row in the year list (own day-off or read-only holiday).
type FreiRowVM struct {
	DateLabel string // "15.06.2026"
	KindLabel string // domain.Kind.LabelDe()
	Hue       string // dayOffHue(kind)
	Label     string
	IsHoliday bool
	Day       string // yyyy-mm-dd for the delete form
}

// FreiKindOption is one selectable kind in the capture form.
type FreiKindOption struct {
	Value domain.Kind
	Label string
}

// freiKinds lists the six user-creatable day-off kinds (holiday is computed,
// never created here), each with its German label from the domain.
func freiKinds() []FreiKindOption {
	kinds := []domain.Kind{
		domain.KindVacation, domain.KindSick, domain.KindFlex,
		domain.KindSpecial, domain.KindChildSick, domain.KindTraining,
	}
	out := make([]FreiKindOption, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, FreiKindOption{Value: k, Label: k.LabelDe()})
	}
	return out
}

// freiDeleteVals builds the hx-vals JSON for a day-off delete confirm action.
func freiDeleteVals(day string) string {
	return fmt.Sprintf(`{"day":%q}`, day)
}
