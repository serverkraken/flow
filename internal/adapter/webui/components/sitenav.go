package components

// NavItem is one navigation destination. LabelKey is an i18n key.
type NavItem struct {
	Key, Href, LabelKey string
}

// PrimaryNav sind die drei Bereiche der Lesesaal-Topbar (Spec §5.1).
func PrimaryNav() []NavItem {
	return []NavItem{
		{"projekte", "/nodes", "nav.projects"},
		{"docs", "/wissen", "nav.wissen"},
		{"zeit", "/zeit", "nav.zeit"},
	}
}

// UtilityNav ist das Avatar-Menü: Frei · Export · Einstellungen.
func UtilityNav() []NavItem {
	return []NavItem{
		{"frei", "/dayoffs", "nav.dayoffs"},
		{"export", "/export", "nav.export"},
		{"einstellungen", "/einstellungen", "nav.settings"},
	}
}

// AreaFor mappt den active-Key einer Seite auf ihren Topbar-Bereich.
// "" (z. B. home) markiert keinen Bereich — die Wortmarke führt zum Schreibtisch.
func AreaFor(active string) string {
	switch active {
	case "projekte":
		return "projekte"
	case "docs":
		return "docs"
	case "zeit", "heute", "woche", "historie", "stats", "frei", "export":
		return "zeit"
	}
	return ""
}
