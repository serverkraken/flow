package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// Eine Karte am Repo zählt über Vorhaben und Engagement mit hoch — sonst
// stünde in der Schiene neben "Privat" eine 0, obwohl darunter 175 Karten
// liegen.
func TestSubtreeDocTotals_CountsUpTheChain(t *testing.T) {
	e := domain.Node{ID: "e1"}
	v := domain.Node{ID: "v1", ParentID: ptrStr("e1")}
	r := domain.Node{ID: "r1", ParentID: ptrStr("v1")}
	docs := []domain.Document{
		{ID: "d1", NodeID: ptrStr("r1")},
		{ID: "d2", NodeID: ptrStr("r1")},
		{ID: "d3", NodeID: ptrStr("v1")},
		{ID: "d4", NodeID: nil}, // freie Karte zählt nirgends
	}
	got := SubtreeDocTotals([]domain.Node{e, v, r}, docs)
	for id, want := range map[string]int{"r1": 2, "v1": 3, "e1": 3} {
		if got[id] != want {
			t.Errorf("%s = %d, erwartet %d", id, got[id], want)
		}
	}
}

// Ein Dokument an einem Knoten, den die Schiene nicht kennt (archiviert,
// fremd), darf den Aufstieg nicht in eine Endlosschleife schicken.
func TestSubtreeDocTotals_UnknownNodeIsHarmless(t *testing.T) {
	got := SubtreeDocTotals([]domain.Node{{ID: "e1"}}, []domain.Document{{ID: "d1", NodeID: ptrStr("ghost")}})
	if got["e1"] != 0 {
		t.Errorf("fremder Knoten darf nicht mitzählen, got %d", got["e1"])
	}
}

func ptrStr(s string) *string { return &s }
