package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocKindStyle(t *testing.T) {
	cases := map[domain.DocumentType]struct{ label, tone string }{
		domain.DocDaily:       {"Täglich", "accent"},
		domain.DocProject:     {"Projekt", "success"},
		domain.DocFree:        {"Frei", "highlight"},
		domain.DocAgent:       {"Agent", "warning"},
		domain.DocMemory:      {"Memory", "warning"},
		domain.DocInstruction: {"Instruction", "warning"},
		domain.DocSkill:       {"Skill", "warning"},
		domain.DocPlan:        {"Plan", "warning"},
	}
	for typ, want := range cases {
		got := DocKindStyle(typ)
		if got.Label != want.label || got.Tone != want.tone {
			t.Errorf("%s: got {%s,%s} want {%s,%s}", typ, got.Label, got.Tone, want.label, want.tone)
		}
		if got.Glyph == "" {
			t.Errorf("%s: empty glyph", typ)
		}
	}
}
