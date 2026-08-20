package webui

// Bestandsfehler, beim Bau der Kasten-Spalte aufgefallen: DocKindStyle hatte
// keinen Fall für DocSpec und DocActiveContext. Beide fielen auf
// "Label: string(t)" durch — die Regal-Reiter der Kasten-Spalte zeigten
// deshalb "spec 63" und "activecontext 1" neben "Plan 80" und "Memory 24".
//
// Der Test deckt ALLE Typen ab, damit der nächste neue Typ nicht wieder
// still als Rohwert in der Oberfläche landet.

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocKindStyle_EveryTypeHasALabel(t *testing.T) {
	all := []domain.DocumentType{
		domain.DocDaily, domain.DocProject, domain.DocFree, domain.DocAgent,
		domain.DocMemory, domain.DocInstruction, domain.DocSkill,
		domain.DocPlan, domain.DocSpec, domain.DocActiveContext,
	}
	for _, typ := range all {
		got := DocKindStyle(typ).Label
		if got == string(typ) {
			t.Errorf("%q fällt auf den Rohwert durch — es fehlt ein Fall in DocKindStyle", typ)
		}
		if got == "" {
			t.Errorf("%q hat kein Label", typ)
		}
	}
}
