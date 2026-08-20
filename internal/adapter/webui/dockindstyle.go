package webui

import "github.com/serverkraken/flow/internal/domain"

// DocKind is the web presentation of a document type.
type DocKind struct {
	Label string
	Glyph string
	Tone  string
}

// DocKindStyle maps document types to the Wissen UI's category treatment.
func DocKindStyle(t domain.DocumentType) DocKind {
	switch t {
	case domain.DocDaily:
		return DocKind{Label: "Täglich", Glyph: "●", Tone: "accent"}
	case domain.DocProject:
		return DocKind{Label: "Projekt", Glyph: "◆", Tone: "success"}
	case domain.DocFree:
		return DocKind{Label: "Frei", Glyph: "○", Tone: "highlight"}
	case domain.DocAgent:
		return DocKind{Label: "Agent", Glyph: "▪", Tone: "warning"}
	case domain.DocMemory:
		return DocKind{Label: "Memory", Glyph: "▪", Tone: "warning"}
	case domain.DocInstruction:
		return DocKind{Label: "Instruction", Glyph: "▪", Tone: "warning"}
	case domain.DocSkill:
		return DocKind{Label: "Skill", Glyph: "▪", Tone: "warning"}
	case domain.DocPlan:
		return DocKind{Label: "Plan", Glyph: "▪", Tone: "warning"}
	case domain.DocSpec:
		// Fehlte: fiel auf den Rohwert "spec" durch und stand so in den
		// Regal-Reitern der Kasten-Spalte neben "Plan" und "Memory".
		return DocKind{Label: "Spec", Glyph: "▪", Tone: "warning"}
	case domain.DocActiveContext:
		// Ebenso — "activecontext" stand roh in der Oberfläche.
		return DocKind{Label: "Kontext", Glyph: "▪", Tone: "warning"}
	default:
		return DocKind{Label: string(t), Glyph: "▪", Tone: "warning"}
	}
}

// IsSystemKind reports whether a type belongs to the agent/system group.
func IsSystemKind(t domain.DocumentType) bool {
	switch t {
	case domain.DocAgent, domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocPlan:
		return true
	default:
		return false
	}
}
