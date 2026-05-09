package worktime

// NoteSuggestion ist ein Eintrag im Kompendium-Note-Attach-Picker.
// ID ist der Wert, den LinkWriter.Add bekommt; Title ist die
// human-readable Anzeige (Frontmatter-Titel mit Fallback auf ID).
type NoteSuggestion struct {
	ID    string
	Title string
}

// NoteLister liefert die jüngsten Kompendium-Notes für den Attach-
// Picker. Implementiert in cmd/flow/main.go als dünner Adapter über
// kompendium/usecase.ListNotes — der Worktime-Screen bleibt damit
// unabhängig von der Kompendium-Subtree-Typenkette (NoteEntry,
// Frontmatter, …).
//
// Wenn Recent eine leere Slice / nil zurückgibt (kein Index, leere
// Notebook, Adapter-Fehler) degradiert der Picker zur reinen ID-
// Eingabe — der User kann eine vollständige ID tippen wie zuvor.
type NoteLister interface {
	Recent(limit int) []NoteSuggestion
}

// NoteReader liefert den rohen Markdown-Body einer Note. Genutzt vom
// integrierten Note-Viewer in Heute (`o`-Key), der die angehängte Note
// inline mit dem flow-eigenen MarkdownRenderer öffnet, statt einen
// externen Viewer in einem tmux-Split zu starten. Implementiert in
// cmd/flow/main.go als Adapter über kompendium ports.NoteStore.Get.
type NoteReader interface {
	Read(id string) (string, error)
}
