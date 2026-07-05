package webui

import "github.com/serverkraken/flow/internal/domain"

// DocTypeChipClass mappt einen Dokumenttyp auf seine feste .tc-*-Chip-Klasse
// (Spec §7.1). Semantisch, überall gleich — NIE pro Projekt.
func DocTypeChipClass(t domain.DocumentType) string {
	switch t {
	case domain.DocPlan, domain.DocActiveContext: // context → violett (Spec §7.1)
		return "tc-v"
	case domain.DocSpec:
		return "tc-t"
	case domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocAgent:
		return "tc-o"
	case domain.DocDaily, domain.DocFree:
		return "tc-g"
	default: // DocProject/notiz + Rest → blau
		return "tc-b"
	}
}

// DocTypeLabel ist der dt. Anzeigename (ohne Glyph) — wiederverwendet DocKindStyle.
func DocTypeLabel(t domain.DocumentType) string { return DocKindStyle(t).Label }
