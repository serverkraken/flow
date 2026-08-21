package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocTypeChipClass(t *testing.T) {
	cases := map[domain.DocumentType]string{
		domain.DocProject:       "tc-b", // Notiz
		domain.DocInstruction:   "tc-b", // Notiz, Instruktion
		domain.DocFree:          "tc-b",
		domain.DocPlan:          "tc-v", // Plan
		domain.DocSkill:         "tc-v", // Ausstattung
		domain.DocSpec:          "tc-t", // Spec
		domain.DocMemory:        "tc-r", // Erinnerung
		domain.DocAgent:         "tc-r",
		domain.DocActiveContext: "tc-o", // Kontext = Akzent
		domain.DocDaily:         "tc-g", // Tagebuch
	}
	for in, want := range cases {
		if got := DocTypeChipClass(in); got != want {
			t.Errorf("DocTypeChipClass(%v) = %q, want %q", in, got, want)
		}
	}
	if DocTypeLabel(domain.DocPlan) == "" {
		t.Fatal("DocTypeLabel must not be empty")
	}
	// Chip-Text nie der rohe Typ-String (Gate-Finding: "spec"/"activecontext").
	labels := map[domain.DocumentType]string{
		domain.DocSpec:          "Spec",
		domain.DocActiveContext: "Kontext",
	}
	for in, want := range labels {
		if got := DocTypeLabel(in); got != want {
			t.Errorf("DocTypeLabel(%v) = %q, want %q", in, got, want)
		}
	}
}
