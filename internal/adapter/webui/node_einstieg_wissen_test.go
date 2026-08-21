package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Wissen vor Zahlen: der Kasten zeigt die frischesten Karten des Teilbaums
// mit Herkunft, gedeckelt auf fünf, und die Typen als Zähler, die den
// Überblick je Ebene filtern.
func TestBuildNodeEinstieg_WissenVorZahlen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	eng1, subtree, allNodes := einstiegFixture()
	var docs []domain.Document
	for i := 0; i < 6; i++ {
		docs = append(docs, domain.Document{ID: "s" + string(rune('a'+i)), NodeID: ptr("repoA"), Type: domain.DocSpec, Title: "Spec " + string(rune('A'+i)), UpdatedAt: now.Add(-time.Duration(i+2) * time.Hour)})
	}
	docs = append(docs, domain.Document{ID: "own", NodeID: ptr("eng1"), Type: domain.DocProject, Title: "Direkt hier", UpdatedAt: now.Add(-time.Hour)})
	docs = append(docs, domain.Document{ID: "arch", NodeID: ptr("repoA"), Type: domain.DocSpec, Title: "Archiv", Archived: true, UpdatedAt: now})
	docs = append(docs, domain.Document{ID: "fremd", NodeID: ptr("foreignNode"), Type: domain.DocSpec, Title: "Fremd", UpdatedAt: now})

	out := BuildNodeEinstieg(context.Background(), EinstiegInput{N: eng1, Ancestors: []domain.Node{eng1}, AllNodes: allNodes, Subtree: subtree, Docs: docs, Now: now})

	if out.WissenHref != "/nodes/eng1?tab=wissen" {
		t.Errorf("WissenHref = %s", out.WissenHref)
	}
	if len(out.WissenRecent) != einstiegWissenCap {
		t.Fatalf("Kasten zeigt %d Karten, want %d", len(out.WissenRecent), einstiegWissenCap)
	}
	if out.WissenRecent[0].ID != "own" || out.WissenRecent[0].Where != "" {
		t.Errorf("die frischeste Karte zuerst, an der Ebene selbst ohne Herkunft: %+v", out.WissenRecent[0])
	}
	if out.WissenRecent[1].Where == "" {
		t.Errorf("eine Karte aus dem Teilbaum nennt ihre Herkunft: %+v", out.WissenRecent[1])
	}
	for _, r := range out.WissenRecent {
		if r.Title == "Archiv" || r.Title == "Fremd" {
			t.Errorf("archivierte und fremde Karten gehören nicht in den Kasten: %s", r.Title)
		}
	}
	if len(out.WissenTypes) != 2 || out.WissenTypes[0].Type != domain.DocSpec || out.WissenTypes[0].Count != 6 {
		t.Fatalf("Typ-Zähler = %+v", out.WissenTypes)
	}
	if out.WissenTypes[0].Href != "/nodes/eng1?tab=wissen&typ=spec" {
		t.Errorf("der Zähler filtert den Überblick: %s", out.WissenTypes[0].Href)
	}
}

// Der Block steht VOR den Kennzahlen — das ist die Umgewichtung.
func TestEinstiegKasten_WissenBeforeNumbers(t *testing.T) {
	t.Parallel()
	eng1, _, _ := einstiegFixture()
	d := NodeEinstieg{N: eng1, WissenHref: "/nodes/eng1?tab=wissen", WeekDecimal: "0,0", MonthStr: "0:00", YearStr: "0:00",
		WissenRecent: []EinstiegWissenRow{{ID: "d1", Title: "Satzspiegel", ChipClass: "tc-t", ChipLabel: "Spec", Where: "repoA", When: "Mo"}},
		WissenTypes:  []WissenTypZaehler{{Type: domain.DocSpec, Label: "Spec", Count: 1, Href: "/nodes/eng1?tab=wissen&typ=spec"}},
	}
	out := renderToBuf(t, context.Background(), EinstiegKasten(d))
	iW, iK := strings.Index(out, `data-einstieg-wissen`), strings.Index(out, `grid-cols-3`)
	if iW < 0 || iK < 0 || iW > iK {
		t.Fatalf("Wissen (%d) muss vor den Kennzahlen (%d) stehen", iW, iK)
	}
	for _, want := range []string{"Satzspiegel", "repoA", `href="/wissen/d1"`, `href="/nodes/eng1?tab=wissen&amp;typ=spec"`, `<span class="tnum">1</span> Spec`} {
		if !strings.Contains(out, want) {
			t.Errorf("Kasten ohne %q", want)
		}
	}
	leer := renderToBuf(t, context.Background(), EinstiegKasten(NodeEinstieg{N: eng1, WissenHref: "/nodes/eng1?tab=wissen"}))
	if !strings.Contains(leer, "Noch keine Karte in diesem Register.") || !strings.Contains(leer, `href="/wissen/neu?node=eng1"`) {
		t.Errorf("der leere Block sagt, was hineinkäme, und führt zum Editor")
	}
}
