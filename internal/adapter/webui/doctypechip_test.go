package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocTypeChipClass(t *testing.T) {
	cases := map[domain.DocumentType]string{
		domain.DocProject:       "tc-b",
		domain.DocPlan:          "tc-v",
		domain.DocSpec:          "tc-t",
		domain.DocMemory:        "tc-o",
		domain.DocDaily:         "tc-g",
		domain.DocFree:          "tc-g",
		domain.DocActiveContext: "tc-v", // Spec §7.1 context → violett
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
