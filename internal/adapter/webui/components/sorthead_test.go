package components_test

// Der Sortierkopf (Katalog 3.10): "Die Spaltenüberschrift ist der Schalter.
// Ein Pfeil zeigt die Richtung, Grau heißt unsortiert. Keine zweite
// Werkzeugleiste." Und: ist eine andere Wahl aktiv, sagt die Überschrift das
// an — sie heißt dann "angelegt", nicht weiter "geändert".

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func headVM(active string) components.SortHeadVM {
	return components.SortHeadVM{
		HeadKey: "sort.head.changed",
		Options: []components.SortHeadOption{
			{LabelKey: "sort.changed", Href: "/wissen", Active: active == "changed"},
			{LabelKey: "sort.created", Href: "/wissen?sort=angelegt", Active: active == "created"},
			{LabelKey: "sort.title", Href: "/wissen?sort=titel", Active: active == "title"},
		},
	}
}

func TestSortHead_IsTheColumnHeadingItself(t *testing.T) {
	out := render(t, components.SortHead(headVM("changed")))
	// Kein zweiter Werkzeugkasten: die Überschrift selbst öffnet die Wahl.
	if !strings.Contains(out, "<summary") {
		t.Errorf("die Überschrift muss der Schalter sein: %s", out)
	}
	if !strings.Contains(out, "▾") {
		t.Errorf("ein Pfeil zeigt die Richtung: %s", out)
	}
	for _, want := range []string{"/wissen?sort=angelegt", "/wissen?sort=titel"} {
		if !strings.Contains(out, want) {
			t.Errorf("die Wahl ist sichtbar, nicht versteckt — %q fehlt: %s", want, out)
		}
	}
}

// Die aktive Wahl trägt das Häkchen — genau eines.
func TestSortHead_MarksExactlyOneChoice(t *testing.T) {
	out := render(t, components.SortHead(headVM("created")))
	if n := strings.Count(out, "✓"); n != 1 {
		t.Errorf("genau ein Häkchen erwartet, got %d: %s", n, out)
	}
}

// R5 des Karteikastens: alles eckig. Kein Radius an Flächen, Knöpfen,
// Feldern, Dialogen.
func TestSortHead_HasNoRoundedCorners(t *testing.T) {
	out := render(t, components.SortHead(headVM("changed")))
	if strings.Contains(out, "rounded") {
		t.Errorf("der Karteikasten ist eckig (R5): %s", out)
	}
}
