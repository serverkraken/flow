package webui

// R3 des Karteikastens: "Ein Zeilenmuster für alle Listen — Karten, Notizen,
// Buchungen, Artefakte, Treffer: dieselbe Zeile, dieselben Polster, dieselbe
// 3px-Schiene links."
//
// Das Tint-System ist die eine Regel dahinter: beim Überfahren trägt die
// Zeile den Wash ihres Tons als Fläche und die 3px-Auswahlkante in der vollen
// Tonfarbe; Auswahl ist derselbe Zustand, nur dauerhaft. Damit das für JEDE
// Liste gilt, muss die gemeinsame .row-Regel es tragen — nicht jede Liste
// einzeln.

import (
	"os"
	"strings"
	"testing"
)

func TestR3_SharedRowCarriesTheSelectionEdge(t *testing.T) {
	b, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, "  .row {")
	if i < 0 {
		t.Fatal(".row-Regel nicht gefunden")
	}
	rule := src[i : i+strings.Index(src[i:], "}")]
	if !strings.Contains(rule, "border-left: 3px solid transparent") {
		t.Errorf("die geteilte Zeile braucht die 3px-Kante (durchsichtig, damit sie ohne Sprung erscheint): %s", rule)
	}

	j := strings.Index(src, "a.row:hover")
	if j < 0 {
		t.Fatal("Hover-Regel der Zeile nicht gefunden")
	}
	hover := src[j : j+strings.Index(src[j:], "}")]
	for _, want := range []string{"--tint-wash", "border-left-color"} {
		if !strings.Contains(hover, want) {
			t.Errorf("das Überfahren muss durch das Tint-System laufen (%q fehlt): %s", want, hover)
		}
	}
}
