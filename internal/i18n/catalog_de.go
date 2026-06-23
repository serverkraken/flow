package i18n

func init() {
	register(DE, catalog{
		strings: map[string]string{
			// brand / shell
			"app.name":            "flow",
			"app.tagline":         "Zeit & Wissen",
			// top-level navigation
			"nav.today":           "Heute",
			"nav.week":            "Woche",
			"nav.history":         "Historie",
			"nav.knowledge":       "Wissen",
			"nav.projects":        "Projekte",
			"nav.stats":           "Stats",
			"nav.dayoffs":         "Frei",
			"nav.export":          "Export",
			"nav.settings":        "Einstellungen",
			"nav.menu":            "Menü",
			"nav.account":         "Konto",
			"nav.logout":          "Abmelden",
			"nav.primary":         "Hauptnavigation",
			// theme toggle
			"theme.toggle":        "Hell/Dunkel umschalten",
			"theme.toLight":       "Zu Hell wechseln",
			"theme.toDark":        "Zu Dunkel wechseln",
			// common buttons / actions
			"common.new":          "Neu",
			"common.save":         "Speichern",
			"common.cancel":       "Abbrechen",
			"common.delete":       "Löschen",
			"common.edit":         "Bearbeiten",
			"common.confirm":      "Bestätigen",
			"common.close":        "Schließen",
			"common.search":       "Suchen…",
			"common.loading":      "Lädt…",
			// pagination
			"page.prev":           "Zurück",
			"page.next":           "Weiter",
			"page.more":           "Mehr laden",
			"page.label":          "Seitennavigation",
			// empty states
			"empty.default":       "Nichts vorhanden",
			// confirm dialog defaults
			"confirm.title":       "Bist du sicher?",
			"confirm.deleteBody":  "Diese Aktion kann nicht rückgängig gemacht werden.",
			// doc kinds (badges)
			"dockind.daily":       "Daily",
			"dockind.project":     "Projekt",
			"dockind.free":        "Frei",
			"dockind.agent":       "Agent",
			// styleguide (only used by the /ui demo page)
			"styleguide.title":    "Design-System",
			"styleguide.subtitle": "Komponenten-Schaukasten",
		},
		plurals: map[string]Plural{
			"list.entries": {One: "{{.N}} Eintrag", Other: "{{.N}} Einträge"},
			"list.results": {One: "{{.N}} Treffer", Other: "{{.N}} Treffer"},
		},
	})
}
