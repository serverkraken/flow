package webui

// Der Karteikasten ist ein DREISPALTER: 264px Schiene · 372px Kasten ·
// 1fr Lesesaal. 19 der 41 Mockup-Screens bauen darauf. Ohne die mittlere
// Spalte ist jede Fläche "Schiene plus eine breite Spalte" — und das ist
// nicht der Karteikasten (Soenne, 2026-08-20).
//
// Die Bibliothek (Screen 06) legt die REGALE in den Kasten; Suche, Filter
// und Trefferliste bleiben im Lesesaal.

import (
	"context"
	"strings"
	"testing"
)

func TestWissen_IsAThreeColumnScreen(t *testing.T) {
	out := renderToBuf(t, context.Background(), WissenFragment(WissenOverviewVM{
		Shelves: []WissenShelf{{TypeKey: "plan", LabelKey: "wissen.shelf.plan", DescKey: "wissen.shelf.plan.desc", Count: 102}},
	}))
	for _, want := range []string{"k3-cols", "372px", "k3-panel"} {
		if !strings.Contains(out, want) {
			t.Errorf("die Bibliothek braucht das Dreispalter-Raster (%q fehlt)", want)
		}
	}
	// Die Regale stehen IM Kasten, nicht im Lesesaal.
	panel := strings.Index(out, "k3-panel")
	shelf := strings.Index(out, "wissen.shelf.plan")
	if shelf < 0 {
		shelf = strings.Index(out, "Pläne")
	}
	if panel < 0 || shelf < 0 || shelf < panel {
		t.Errorf("die Regale gehören in die Kasten-Spalte (panel@%d, Regal@%d)", panel, shelf)
	}
}
