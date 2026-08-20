package webui

// Slice 4 — der Lesesaal. TOKENS.md, Typo-Leiter:
//
//	16,5–18px / 1,82   Lesetext (16,5 Karte, 18 README/Langtext)
//	20 / 750 / −.015em Artikelabschnitt (H2)
//	19 / 750           Unterabschnitt (H3)
//
// Satzbreite 660–720px. "Kein Element darf die Spalte verbreitern" (R6).

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func proseCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	// Auf die BASISREGEL ankern, nicht auf die erste .prose-Zeile der Datei:
	// die Media-Queries tragen eigene .prose-Blöcke, und der erste Treffer war
	// einer davon. Nur die Basisregel führt overflow-wrap.
	for i := 0; ; {
		j := strings.Index(src[i:], ".prose {")
		if j < 0 {
			t.Fatal("Basisregel .prose nicht gefunden")
		}
		start := i + j
		end := start + strings.Index(src[start:], "}") + 1
		block := src[start:end]
		if strings.Contains(block, "overflow-wrap") {
			return block
		}
		i = end
	}
}

func TestLesesaal_ReadingMeasure(t *testing.T) {
	block := proseCSS(t)
	for _, want := range []string{"font-size: 16.5px", "line-height: 1.82"} {
		if !strings.Contains(block, want) {
			t.Errorf("Lesetext braucht %q (TOKENS.md Typo-Leiter): %s", want, block)
		}
	}
	m := regexp.MustCompile(`max-width: (\d+)px`).FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("keine Satzbreite gesetzt: %s", block)
	}
	w, _ := strconv.Atoi(m[1])
	if w < 660 || w > 720 {
		t.Errorf("Satzbreite %dpx liegt außerhalb 660–720px", w)
	}
}

func TestLesesaal_HeadingLadder(t *testing.T) {
	b, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, c := range []struct{ sel, size string }{
		{".prose h2 {", "font-size: 20px"},
		{".prose h3 {", "font-size: 19px"},
	} {
		i := strings.Index(src, c.sel)
		if i < 0 {
			t.Fatalf("%s fehlt", c.sel)
		}
		rule := src[i : i+220]
		if !strings.Contains(rule, c.size) {
			t.Errorf("%s braucht %q: %s", c.sel, c.size, rule)
		}
		if !strings.Contains(rule, "font-weight: 750") {
			t.Errorf("%s: Überschriften tragen Gewicht 750: %s", c.sel, rule)
		}
	}
}

// Ein ARBEITENDER Einbettungszustand ist Maschinerie und gehört nicht in den
// Kopf eines Artikels — dort stand "Eingebettet" direkt unter dem Titel. Nur
// ein Fehlschlag verlangt etwas vom Leser (Katalog 3.6, Rot-Reserve).
func TestLesesaal_EmbedStateOnlyOnFailure(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "T", Embed: &EmbedView{State: "ok"}}
	if out := renderDoc(t, vm); strings.Contains(out, "Eingebettet") {
		t.Errorf("ein arbeitender Einbettungszustand gehört nicht in den Artikelkopf: %.300s", out)
	}
	vm.Embed = &EmbedView{State: "failed", ShowRetry: true}
	if out := renderDoc(t, vm); !strings.Contains(out, "fehlgeschlagen") {
		t.Errorf("ein Fehlschlag muss sichtbar bleiben: %.300s", out)
	}
}

func renderDoc(t *testing.T, vm DocumentVM) string {
	t.Helper()
	return renderToBuf(t, context.Background(), DocumentFragment(vm))
}
