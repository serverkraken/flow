package worktime

import "github.com/serverkraken/flow/internal/frontend/tui/components/help"

// HelpSections returns the canonical key-binding inventory for the
// worktime screen, suitable for direct embedding in the sidekick
// `?`-overlay aggregator. Concatenates: tab-router, every sub-tab's
// own bindings (heute via helpSectionsHeute / woche / history / frei),
// and the action menu.
//
// Single source of truth: the same data drives the standalone
// heute-help (worktime/today.go renderHelpRows) and any future
// per-tab `?`-overlay; consumers must not maintain a parallel copy.
func (Model) HelpSections() []help.Section {
	out := []help.Section{helpSectionsTabs()}
	out = append(out, helpSectionsHeute()...)
	out = append(out, helpSectionsWoche()...)
	out = append(out, helpSectionsHistory()...)
	out = append(out, helpSectionsFrei()...)
	out = append(out, helpSectionsMenu())
	return out
}

// helpSectionsTabs covers the worktime root's tab-switching keys plus
// the global `:` action-menu trigger. These keys live at the
// worktime-root level (worktime/model.go's handleTabRouterKey + the
// menu open).
func helpSectionsTabs() help.Section {
	return help.Section{
		Title: "Worktime — Tabs",
		Keys: [][2]string{
			{"1 · 2 · 3 · 4", "Heute · Woche · History · Frei"},
			{"Tab", "Nächster Tab"},
			{"b", "Zurück zur Palette"},
			{":", "Aktions-Menü (Brief · Export · Stats · Korrektur · Land)"},
			{"q", "Beenden — auch aus Dialogen / Aktions-Menü heraus"},
		},
	}
}

// helpSectionsWoche enumerates the woche-tab key bindings.
func helpSectionsWoche() []help.Section {
	return []help.Section{{
		Title: "Worktime — Woche",
		Keys: [][2]string{
			{"j/k · g/G", "Tag fokussieren · oben/unten"},
		},
	}}
}

// helpSectionsHistory enumerates the history-tab bindings (list /
// heatmap / tag-clock / month modes share the same key surface).
func helpSectionsHistory() []help.Section {
	return []help.Section{{
		Title: "Worktime — History",
		Keys: [][2]string{
			{"j/k · g/G", "Cursor / Zeile · oben/unten"},
			{"Enter", "Drill-Down auf den Tag"},
			{"v", "Ansicht: Liste → Heatmap → Tag-Clock → Monat"},
			{"/", "Filter (KW18, 2026, 2026-04, tag:deep, note:standup)"},
			{"F", "Filter mit Prefix »tag:« vorbelegen"},
			{"[ / ]", "Filter um eine Einheit zurück / vor"},
			{"T", "Filter zurücksetzen / aktuelles Fenster"},
			{"h · l (Heatmap/Tag-Clock/Monat)", "Cursor horizontal"},
		},
	}}
}

// helpSectionsFrei enumerates the dayoffs-tab bindings.
func helpSectionsFrei() []help.Section {
	return []help.Section{{
		Title: "Worktime — Frei",
		Keys: [][2]string{
			{"j/k · g/G", "Eintrag fokussieren"},
			{"a", "Tag(e) frei eintragen (Form)"},
			{"A · K", "heute = Urlaub · heute = krank"},
			{"B", "Gesetzliche Feiertage syncen (Default-Land)"},
			{"D", "Eintrag löschen (y/Enter bestätigt)"},
			{"h · l · [ · ]", "Jahr zurück / vor"},
			{"T", "Aktuelles Jahr"},
		},
	}}
}

// helpSectionsMenu describes the worktime action menu (`:`-popup).
func helpSectionsMenu() help.Section {
	return help.Section{
		Title: "Worktime — Aktions-Menü (`:`)",
		Keys: [][2]string{
			{"Brief Wochen-/Monatsbericht", "Markdown via glow / clipboard / ~/Downloads"},
			{"Export CSV / JSON", "Range-Form + Output-Target"},
			{"Stats für Range", "Aggregate über StatsComputer"},
			{"Startzeit korrigieren", "Heute, nur wenn Session läuft"},
			{"Land für Feiertage", "Bundesland-Picker → SyncGermanHolidays"},
			{"j/k · g/G · enter · esc", "Im Menü navigieren / picken / abbrechen"},
			{"tippen", "Live-Filter über die Aktions-Liste"},
			{"c · s · f", "Output-Target direkt: Clipboard / Split / Datei"},
		},
	}
}
