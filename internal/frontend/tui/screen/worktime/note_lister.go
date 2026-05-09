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
