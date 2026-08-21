package webui

import "strings"

// ErrorVM treibt die Fehlerseiten (Screen 20): ein Code als Marke, ein Satz,
// der sagt, was los ist, und ein oder zwei Wege weiter. Keine Schuld, keine
// Technik im Vordergrund — die Fehler-ID steht klein dabei, damit sie sich
// melden lässt.
type ErrorVM struct {
	Status   int
	Code     string // "404", "⌀", "500" — die große Marke
	TitleKey string
	MsgKey   string
	Path     string // der angefragte Pfad, als Adresse gezeigt
	ErrID    string // nur beim Serverfehler
	User     string // Anzeigename, für „Angemeldet als …"
	Ways     []ErrorWay
}

// ErrorWay ist ein Weg weiter — Knopf oder Link.
type ErrorWay struct {
	LabelKey string
	Href     string
	Primary  bool
	Reload   bool // lädt die Seite neu statt zu navigieren
	Search   bool // öffnet die Suche (⌘K)
}

// NotFoundVM baut die 404 — und sagt, WAS fehlt: eine Karte unter /wissen,
// ein Register unter /nodes, sonst eine Seite.
func NotFoundVM(path, user string) ErrorVM {
	vm := ErrorVM{Status: 404, Code: "404", Path: path, User: user, TitleKey: "error.404.title", MsgKey: "error.404.msg"}
	switch {
	case strings.HasPrefix(path, "/wissen/"):
		vm.TitleKey, vm.MsgKey = "error.404.card.title", "error.404.card.msg"
		vm.Ways = []ErrorWay{{LabelKey: "error.way.library", Href: "/wissen", Primary: true}, {LabelKey: "error.way.search", Search: true}}
	case strings.HasPrefix(path, "/nodes/") || strings.HasPrefix(path, "/kontext/"):
		vm.TitleKey, vm.MsgKey = "error.404.node.title", "error.404.node.msg"
		vm.Ways = []ErrorWay{{LabelKey: "error.way.nodes", Href: "/nodes", Primary: true}, {LabelKey: "error.way.search", Search: true}}
	default:
		vm.Ways = []ErrorWay{{LabelKey: "error.way.home", Href: "/", Primary: true}, {LabelKey: "error.way.search", Search: true}}
	}
	return vm
}

// ServerErrorVM baut die 500: Daten sind sicher, die Uhr zählt serverseitig
// weiter, die ID lässt sich kopieren.
func ServerErrorVM(path, user, errID string) ErrorVM {
	return ErrorVM{
		Status: 500, Code: "500", Path: path, User: user, ErrID: errID,
		TitleKey: "error.500.title", MsgKey: "error.500.msg",
		Ways: []ErrorWay{{LabelKey: "error.way.reload", Reload: true, Primary: true}, {LabelKey: "error.way.home", Href: "/"}},
	}
}

// ForbiddenVM baut „Kein Zugriff": das Konto ist nicht freigeschaltet.
func ForbiddenVM(path, user string) ErrorVM {
	return ErrorVM{
		Status: 403, Code: "⌀", Path: path, User: user,
		TitleKey: "error.403.title", MsgKey: "error.403.msg",
		Ways: []ErrorWay{{LabelKey: "error.way.home", Href: "/", Primary: true}},
	}
}
