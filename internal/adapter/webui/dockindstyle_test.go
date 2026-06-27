package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestProjectSwatchStyle(t *testing.T) {
	if s := projectSwatchStyle(""); s != "" {
		t.Errorf("projectSwatchStyle('') = %q, want ''", s)
	}
	got := projectSwatchStyle("#ff0000")
	if got != "background-color: #ff0000" {
		t.Errorf("projectSwatchStyle('#ff0000') = %q", got)
	}
}

func TestIsSystemKind(t *testing.T) {
	for _, system := range []domain.DocumentType{domain.DocAgent, domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocPlan} {
		if !IsSystemKind(system) {
			t.Errorf("IsSystemKind(%q) = false, want true", system)
		}
	}
	for _, nonSystem := range []domain.DocumentType{domain.DocDaily, domain.DocProject, domain.DocFree} {
		if IsSystemKind(nonSystem) {
			t.Errorf("IsSystemKind(%q) = true, want false", nonSystem)
		}
	}
}

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
