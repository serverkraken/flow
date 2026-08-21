package webui

import "github.com/serverkraken/flow/internal/domain"

// DocTypeChipClass ordnet jedem Kartentyp seinen Ton aus TOKENS.md
// (Kartentypen und Bereiche) zu: Plan und Skill violett, Spec petrol,
// Memory rot (Erinnerung), Kontext im Akzent, Tagebuch grün, alles
// Geschriebene ohne eigene Farbe blau (Notiz). Feste Zuordnung, nie pro
// Projekt.
func DocTypeChipClass(t domain.DocumentType) string {
	switch t {
	case domain.DocPlan, domain.DocSkill: // Plan #7A4FD0 — Vorhaben, Ausstattung
		return "tc-v"
	case domain.DocSpec: // Spec #0B8A7B — Festlegungen
		return "tc-t"
	case domain.DocMemory, domain.DocAgent: // Erinnerung #B4452F
		return "tc-r"
	case domain.DocActiveContext: // Kontext #B8720F — der Akzent
		return "tc-o"
	case domain.DocDaily: // Tagebuch #5A7A2E
		return "tc-g"
	default: // Notiz #3D5EDB — Projekt, Instruktion, Frei
		return "tc-b"
	}
}

// DocTypeLabel ist der dt. Anzeigename (ohne Glyph) — wiederverwendet
// DocKindStyle, überschreibt aber die Typen, für die dessen default auf den
// rohen Typ-String zurückfällt ("spec"/"activecontext" als Chip-Text).
func DocTypeLabel(t domain.DocumentType) string {
	switch t {
	case domain.DocSpec:
		return "Spec"
	case domain.DocActiveContext:
		return "Kontext"
	}
	return DocKindStyle(t).Label
}
