package worktime

import (
	"strings"
)

// menuContext bündelt den Snapshot, gegen den das Aktions-Menü die
// Sichtbarkeit von kontext-abhängigen Einträgen entscheidet. Wird beim
// Öffnen des Menüs (`:`) einmal aus deps.Reader gezogen und dann pro
// Filter-Tastendruck ungeändert wiederverwendet — vorher rief der
// Predicate auf jedem Keystroke synchron Reader.Today() (= TSV-Read).
type menuContext struct {
	activeTab    tab
	todayRunning bool
}

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

// menuAction is one entry in the action menu. Sichtbarkeit wird per
// menuActionKind in isMenuActionVisible entschieden — kein per-Eintrag-
// Predicate-Closure mehr, das pro Keystroke ausgewertet werden muss.
type menuAction struct {
	kind    menuActionKind
	section string
	label   string
	hint    string
}

const (
	menuSectionContext = "aktiver tab"
	menuSectionGeneral = "allgemein"
)

// menuActionRegistry ist die statische Liste aller registrierten Aktionen.
// Vorher allozierte allMenuActions() das Slice + den Predicate-Closure
// pro Keystroke neu — bei aktivem Filter bedeutete das mehrere Closure-
// Allokationen und einen Reader.Today()-Read pro getippter Taste. Liste
// ist immutabel, daher als package-var sicher.
//
// Der Land-Hint wird in computeMenuActions aus Deps.Land aufgelöst —
// das Land kommt seit A1 (Env-Var-Disziplin) als Wiring-Parameter rein
// (cmd/flow), nicht mehr aus os.Getenv im Screen-Code.
var menuActionRegistry = []menuAction{
	{
		kind:    menuActionCorrect,
		section: menuSectionContext,
		label:   "Startzeit der laufenden Session korrigieren",
		hint:    "HH:MM",
	},
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
		// hint wird in computeMenuActions aus landOrDefault(deps.Land) befüllt.
	},
}

// isMenuActionVisible wertet die Kontext-Sichtbarkeit aus. Vorher in
// einer Closure pro Eintrag, jetzt in einem zentralen Switch — eine
// neue kontext-abhängige Aktion fügt sich hier mit einem Case-Zweig
// ein. Synchroner I/O hat hier nichts zu suchen; ctx ist ein
// vorab-gezogener Snapshot.
func isMenuActionVisible(kind menuActionKind, ctx menuContext) bool {
	switch kind {
	case menuActionCorrect:
		return ctx.activeTab == tabHeute && ctx.todayRunning
	}
	return true
}

// computeMenuActions filtert die gecachte Action-Registry nach Kontext
// und Filter-Query. Allocates nur das `out`-Slice; das Action-Slice
// selbst ist statisch.
func computeMenuActions(ctx menuContext, query, land string) []menuAction {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]menuAction, 0, len(menuActionRegistry))
	for _, a := range menuActionRegistry {
		if !isMenuActionVisible(a.kind, ctx) {
			continue
		}
		// menuActionLand-Hint zeigt das aktuell konfigurierte
		// Bundesland — Wert kommt aus Deps.Land (vom Composition Root
		// aus $WORKTIME_LAND aufgelöst, A1).
		if a.kind == menuActionLand {
			a.hint = "aktuell: " + landOrDefault(land)
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

// landOrDefault applies the documented "NW" fallback when the
// composition root passed an empty land string. Mirrors the previous
// os.Getenv("WORKTIME_LAND") + NW-default contract; centralised so the
// menu hint and dayoff sync use exactly the same resolution.
func landOrDefault(land string) string {
	if land != "" {
		return land
	}
	return "NW"
}
