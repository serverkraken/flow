package worktime

import (
	"os"
	"strings"
)

// menuActionKind discriminates what runAction does after the user
// confirms a menu entry. Slice B routes every kind through the shared
// TODO-toast in runAction; Slice C/D/E grow runAction into a switch
// over this enum as the real flows arrive.
type menuActionKind int

const (
	// menuActionBriefWeek opens the brief flow with the current week
	// fixed as the range, then routes through the output target
	// sub-picker. Slice C.
	menuActionBriefWeek menuActionKind = iota
	// menuActionBriefMonth — same as BriefWeek but for the current month.
	menuActionBriefMonth
	// menuActionExportCSV / JSON open the export flow (range form,
	// then output target sub-picker). Slice D.
	menuActionExportCSV
	menuActionExportJSON
	// menuActionStats opens the stats flow (range form, then output
	// target sub-picker). Slice D.
	menuActionStats
	// menuActionLand opens the Bundesland picker; the picked Land
	// triggers DayOffWriter.SyncGermanHolidays for the current year
	// inline. Always visible — same flow whether the user came from
	// the Frei tab or any other surface.
	menuActionLand
	// menuActionCorrect opens the HH:MM form for the running session's
	// start time. Heute-only, gated by predicate. Slice E.
	menuActionCorrect
)

// menuAction is one entry in the action menu. predicate decides whether
// it shows in the current context (active tab + worktime state); a nil
// predicate means always-visible.
type menuAction struct {
	kind      menuActionKind
	section   string
	label     string
	hint      string
	predicate func(activeTab tab, deps Deps) bool
}

const (
	menuSectionContext = "aktiver tab"
	menuSectionGeneral = "allgemein"
)

// allMenuActions returns the global registry. Order is render order:
// context section first when applicable, then general. Predicates filter
// against (activeTab, deps); a nil predicate means always-visible.
func allMenuActions() []menuAction {
	return []menuAction{
		// — context-specific —
		{
			kind:    menuActionCorrect,
			section: menuSectionContext,
			label:   "Startzeit der laufenden Session korrigieren",
			hint:    "HH:MM",
			predicate: func(activeTab tab, deps Deps) bool {
				if activeTab != tabHeute {
					return false
				}
				if deps.Reader == nil {
					return false
				}
				day, err := deps.Reader.Today()
				if err != nil {
					return false
				}
				return day.IsRunning()
			},
		},
		// — general (always visible) —
		{
			kind:    menuActionBriefWeek,
			section: menuSectionGeneral,
			label:   "Brief Wochenbericht",
			hint:    "aktuelle KW · Markdown",
		},
		{
			kind:    menuActionBriefMonth,
			section: menuSectionGeneral,
			label:   "Brief Monatsbericht",
			hint:    "aktueller Monat · Markdown",
		},
		{
			kind:    menuActionExportCSV,
			section: menuSectionGeneral,
			label:   "Export CSV",
			hint:    "Range wählbar",
		},
		{
			kind:    menuActionExportJSON,
			section: menuSectionGeneral,
			label:   "Export JSON",
			hint:    "Range wählbar",
		},
		{
			kind:    menuActionStats,
			section: menuSectionGeneral,
			label:   "Stats für Range",
			hint:    "Range wählbar",
		},
		{
			kind:    menuActionLand,
			section: menuSectionGeneral,
			label:   "Land für Feiertage",
			hint:    "aktuell: " + currentLand(),
		},
	}
}

// computeMenuActions filters allMenuActions by predicate + filter query.
// Empty query means "show everything that the predicate admits".
// Matching is case-insensitive substring on label or hint.
func computeMenuActions(activeTab tab, deps Deps, query string) []menuAction {
	q := strings.ToLower(strings.TrimSpace(query))
	all := allMenuActions()
	out := make([]menuAction, 0, len(all))
	for _, a := range all {
		if a.predicate != nil && !a.predicate(activeTab, deps) {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(a.label), q) &&
				!strings.Contains(strings.ToLower(a.hint), q) {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// currentLand reads WORKTIME_LAND with NW as the documented default.
// Mirrors dayoffs.syncHolidaysCmd's fallback so the menu hint stays
// in sync with what `B` would actually pass to SyncGermanHolidays.
func currentLand() string {
	if v := os.Getenv("WORKTIME_LAND"); v != "" {
		return v
	}
	return "NW"
}
