package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// seedKontextScreen: eine Instruction (Immer-Tier) und zwei Memories im Rang.
func seedKontextScreen(t *testing.T, c *cockpitTestServer) {
	t.Helper()
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Slug: "flow", Name: "flow", Kind: domain.KindRepo})
	nid := "n1"
	for _, d := range []domain.Document{
		{ID: "ins-1", Type: domain.DocInstruction, Path: "claude-md", Title: "Regeln für Agenten", Body: "# Regeln\n\nNie `make fmt` laufen lassen."},
		{ID: "mem-1", Type: domain.DocMemory, Path: "mem-1", Title: "Tailwind v4 gotchas", Body: "Tailwind scannt Doc-Kommentare."},
		{ID: "mem-2", Type: domain.DocMemory, Path: "mem-2", Title: "htmx ohne Skripte", Body: "allowScriptTags ist aus."},
	} {
		d.OwnerID, d.NodeID, d.CreatedAt, d.UpdatedAt, d.UpdatedByRef = "u1", &nid, c.clk.Now(), c.clk.Now(), "msoent"
		if _, err := c.ds.Create(context.Background(), d); err != nil {
			t.Fatalf("seed %s: %v", d.ID, err)
		}
	}
}

// Screen 07: kuratieren links, lesen und bearbeiten rechts. Ohne Wahl zeigt
// die Lesespalte die erste Karte des Immer-Tiers.
func TestWebKontextScreen_ReadsTheFirstCardByDefault(t *testing.T) {
	c := newCockpitTestServer(t)
	seedKontextScreen(t, c)

	rec := c.do(t, "GET", "/kontext/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="kontext-fragment"`,
		`id="kontext-lese"`,
		`hx-get="/kontext/n1/lese?doc=ins-1"`, // die Lesespalte lädt sich mit ihrer Wahl neu
		`hx-get="/kontext/n1?doc=ins-1"`,      // die Liste ebenso
		`data-kontext-lese="ins-1"`,
		"Regeln für Agenten",
		"Nie <code>make fmt</code> laufen lassen.", // gerendert, nicht roh
		`href="/wissen/ins-1/bearbeiten"`,
		`href="/wissen/ins-1"`,
		"immer enthalten",
		`data-kontext-row="ins-1" aria-current`, // die gewählte Zeile ist markiert
		`href="/kontext/n1?doc=mem-1"`,          // jede Zeile wählt
		`hx-post="/kontext/n1/mode?sel=ins-1"`,  // Aktionen tragen die Wahl weiter
		"Im Kontext", "Token",
		`href="/nodes/n1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Screen 07 ohne %q:\n%.2500s", want, body)
		}
	}
}

func TestWebKontextScreen_SelectsByQueryAndServesTheReadingFragment(t *testing.T) {
	c := newCockpitTestServer(t)
	seedKontextScreen(t, c)

	rec := c.do(t, "GET", "/kontext/n1?doc=mem-2", nil)
	body := rec.Body.String()
	if !strings.Contains(body, `data-kontext-lese="mem-2"`) || !strings.Contains(body, "allowScriptTags ist aus.") {
		t.Errorf("?doc= wählt die Karte der Lesespalte: %.1500s", body)
	}
	if !strings.Contains(body, `data-kontext-row="mem-2" aria-current`) || strings.Contains(body, `data-kontext-row="ins-1" aria-current`) {
		t.Errorf("genau die gewählte Zeile ist markiert")
	}
	if !strings.Contains(body, "enthalten · 02/02") && !strings.Contains(body, "enthalten · 01/02") {
		t.Errorf("eine Rang-Karte nennt ihren Rang: %.1500s", body)
	}

	frag := c.do(t, "GET", "/kontext/n1/lese?doc=mem-1", nil)
	fb := frag.Body.String()
	if frag.Code != http.StatusOK || strings.Contains(fb, "<html") || !strings.Contains(fb, `data-kontext-lese="mem-1"`) {
		t.Errorf("die Lese-Route liefert nur die Spalte: %d %.600s", frag.Code, fb)
	}

	// Eine unbekannte Wahl fällt auf den Standard, statt zu brechen.
	fallback := c.do(t, "GET", "/kontext/n1?doc=gibtsnicht", nil)
	if fallback.Code != http.StatusOK || !strings.Contains(fallback.Body.String(), `data-kontext-lese="ins-1"`) {
		t.Errorf("unbekannte Wahl → erste Karte")
	}
}

// Der alte Tab bleibt als Adresse gültig und landet auf Screen 07.
func TestWebKontextScreen_OldTabLandsHere(t *testing.T) {
	c := newCockpitTestServer(t)
	seedKontextScreen(t, c)
	rec := c.do(t, "GET", "/nodes/n1?tab=kontext", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="kontext-lese"`) {
		t.Errorf("?tab=kontext muss Screen 07 zeigen: %d", rec.Code)
	}
}

// Eine Aktion aus der Liste behält die Wahl: der Tausch liefert das
// Fragment mit derselben markierten Zeile.
func TestWebKontextScreen_ActionsKeepTheSelection(t *testing.T) {
	c := newCockpitTestServer(t)
	seedKontextScreen(t, c)
	rec := c.do(t, "POST", "/kontext/n1/mode?sel=mem-2", map[string]string{"doc": "mem-1", "mode": "nie"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-kontext-row="mem-2" aria-current`) {
		t.Errorf("die Wahl überlebt die Aktion: %.1200s", body)
	}
	if strings.Contains(body, `id="kontext-lese"`) {
		t.Errorf("eine Aktion liefert nur das Fragment")
	}
}
