package webui

// Slice 2 — die Schiene (K3Rail). Zwei Sektionen: "01 — REGISTER" ist der
// Knotenbaum mit Ebenen-Einzug und Kartenzähler, "02 — ALLGEMEIN" sind die
// Bereiche, die quer über alle Register laufen. Das ist laut Konzept 02 das
// GANZE Navigationsmodell — es gibt keine zweite Hierarchie.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func railFixture() RailNavVM {
	return RailNavVM{
		Active: "docs",
		Rows: []RailNavRow{
			{ID: "e1", Name: "Privat", Kind: domain.KindEngagement, Level: 0, Cards: 305, HasChildren: true},
			{ID: "v1", Name: "ci-plattform", Kind: domain.KindVorhaben, Level: 1, ParentID: "e1", Cards: 22},
			{ID: "r1", Name: "flow", Kind: domain.KindRepo, Level: 2, ParentID: "v1", Cards: 175},
		},
		BibCount: 440,
	}
}

func TestRailNav_RendersBothSections(t *testing.T) {
	html := renderRail(t, railFixture())
	for _, want := range []string{"01 — REGISTER", "02 — ALLGEMEIN"} {
		if !strings.Contains(html, want) {
			t.Errorf("Schiene braucht Sektion %q", want)
		}
	}
}

func TestRailNav_RegisterTreeCarriesIndentAndCounts(t *testing.T) {
	html := renderRail(t, railFixture())
	for _, want := range []string{"Privat", "ci-plattform", "flow", "/nodes/e1", "/nodes/v1", "/nodes/r1", "305", "22", "175"} {
		if !strings.Contains(html, want) {
			t.Errorf("Registerbaum vermisst %q", want)
		}
	}
	// Einzug je Ebene: 21 / 38 / 55px (K3Rail).
	for _, want := range []string{"pl-[21px]", "pl-[38px]", "pl-[55px]"} {
		if !strings.Contains(html, want) {
			t.Errorf("Ebenen-Einzug %q fehlt", want)
		}
	}
}

func TestRailNav_AllgemeinMarksTheCurrentArea(t *testing.T) {
	html := renderRail(t, railFixture())
	i := strings.Index(html, `href="/wissen"`)
	if i < 0 {
		t.Fatalf("Bibliothek-Zeile fehlt: %.400s", html)
	}
	if !strings.Contains(html[maxInt(0, i-200):minInt(i+260, len(html))], "nv-row-active") {
		t.Errorf("die Bibliothek muss auf ihrer eigenen Seite markiert sein")
	}
}

// Ein Knoten ohne Kinder trägt keinen Pfeil — der Pfeil ist die Antwort auf
// die Frage, ob darunter noch eine Ebene liegt (Konzept 02 B).
func TestRailNav_CaretOnlyWhereChildrenExist(t *testing.T) {
	html := renderRail(t, railFixture())
	if strings.Count(html, `data-nv-caret="`) != 1 {
		t.Errorf("genau ein Pfeil erwartet (nur Privat hat Kinder), got %d", strings.Count(html, `data-nv-caret="`))
	}
}

func renderRail(t *testing.T, vm RailNavVM) string {
	t.Helper()
	var b strings.Builder
	if err := RailNav(vm).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Das Fragment darf KEIN Inline-Skript tragen: dieses Repo fährt htmx mit
// allowScriptTags = false, ein Skript im nachgeladenen Baum wird nie
// ausgeführt. Genau daran ist der erste Bau der Schiene gescheitert — sie
// stand vollständig da und klappte trotzdem nicht auf, ohne eine einzige
// Fehlermeldung. Das Verhalten lebt jetzt in static/js/railnav.js.
func TestRailNav_FragmentCarriesNoInlineScript(t *testing.T) {
	html := renderRail(t, railFixture())
	if strings.Contains(html, "<script") {
		t.Errorf("das Fragment darf kein Inline-Skript tragen (htmx allowScriptTags=false): %.200s", html[strings.Index(html, "<script"):])
	}
}
