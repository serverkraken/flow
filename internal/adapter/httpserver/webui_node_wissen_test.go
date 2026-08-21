package httpserver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// seedWissenTree baut ein Vorhaben mit zwei Repos und Karten: das Vorhaben
// selbst bleibt leer (so sah der Strassenfuchs-Teilbaum am 21.08. aus).
func seedWissenTree(t *testing.T, c *cockpitTestServer) {
	t.Helper()
	c.seedNode(t, domain.Node{ID: "vor", OwnerID: "u1", Slug: "vor", Name: "Strassenfuchs", Kind: domain.KindVorhaben})
	vid := "vor"
	c.seedNode(t, domain.Node{ID: "sf", OwnerID: "u1", Slug: "sf", Name: "strassenfuchs", Kind: domain.KindRepo, ParentID: &vid})
	c.seedNode(t, domain.Node{ID: "dash", OwnerID: "u1", Slug: "dash", Name: "admin-dashboard", Kind: domain.KindRepo, ParentID: &vid})
	now := c.clk.Now()
	mk := func(id, node string, typ domain.DocumentType, title string, age time.Duration) domain.Document {
		n := node
		return domain.Document{ID: id, OwnerID: "u1", NodeID: &n, Type: typ, Path: strings.ReplaceAll(strings.ToLower(title), " ", "-"), Title: title, Body: "# " + title, CreatedAt: now.Add(-age), UpdatedAt: now.Add(-age)}
	}
	for i := 0; i < 10; i++ {
		_, _ = c.ds.Create(context.Background(), mk(fmt.Sprintf("s%02d", i), "sf", domain.DocSpec, fmt.Sprintf("Spec %02d", i), time.Duration(i+2)*time.Hour))
	}
	_, _ = c.ds.Create(context.Background(), mk("p1", "sf", domain.DocPlan, "Plan Eins", 20*time.Hour))
	mem := mk("m1", "sf", domain.DocMemory, "Fallen der Linie", 90*time.Minute)
	mem.ContextMode = domain.ContextModeImmer
	_, _ = c.ds.Create(context.Background(), mem)
	_, _ = c.ds.Create(context.Background(), mk("d1", "dash", domain.DocProject, "Dashboard README", time.Hour))
	arch := mk("a1", "sf", domain.DocSpec, "Alte Spec", time.Hour)
	_, _ = c.ds.Create(context.Background(), arch)
	_ = c.ds.SetArchived(context.Background(), "u1", "a1", true)
}

func TestNodeWissen_GroupsByOriginWithTypeCounters(t *testing.T) {
	c := newCockpitTestServer(t)
	seedWissenTree(t, c)

	rec := c.do(t, "GET", "/nodes/vor?tab=wissen", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="wissen-ebene"`,
		`hx-get="/nodes/vor/wissen"`, // Neulade-Link im Standardzustand
		`href="/nodes/sf"`, "strassenfuchs", "12 Karten",
		`href="/nodes/dash"`, "admin-dashboard", "1 Karte",
		"direkt hier", "Noch keine Karte direkt an dieser Ebene.",
		`href="/nodes/vor?in=sf&amp;tab=wissen&amp;typ=spec"`, `<span class="tnum">10</span> Spec`,
		"Dashboard README", "Spec 00",
		`open=sf`, "alle 12 ›",
		`class="wmark wmark-immer"`, `href="/kontext/vor"`,
		`href="/nodes/vor"`, // der Weg zurück
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Überblick ohne %q:\n%.3000s", want, body)
		}
	}
	if strings.Contains(body, "Alte Spec") {
		t.Errorf("archivierte Karten gehören nicht in den Überblick")
	}
	if strings.Contains(body, "Spec 09") {
		t.Errorf("die ungefilterte Gruppe zeigt anfangs acht Zeilen, nicht alle")
	}
	// Frische: admin-dashboard (1h) vor strassenfuchs (2h), das leere Vorhaben am Ende.
	iDash, iSf, iSelf := strings.Index(body, `data-wissen-gruppe="dash"`), strings.Index(body, `data-wissen-gruppe="sf"`), strings.Index(body, `data-wissen-gruppe="vor"`)
	if iDash >= iSf || iSf >= iSelf {
		t.Errorf("Gruppen nach Frische, die leere eigene Ebene am Ende: dash=%d sf=%d self=%d", iDash, iSf, iSelf)
	}
}

func TestNodeWissen_FragmentCarriesItsState(t *testing.T) {
	c := newCockpitTestServer(t)
	seedWissenTree(t, c)

	req, _ := http.NewRequest("GET", "/nodes/vor/wissen?open=sf&sort=titel&q=spec", nil)
	req.Header.Set("HX-Request", "true")
	cookieVal, _ := c.codec.Issue("u1")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("ein htmx-Aufruf bekommt nur das Fragment")
	}
	if !strings.Contains(body, `hx-get="/nodes/vor/wissen?open=sf&amp;q=spec&amp;sort=titel"`) {
		t.Errorf("der Neulade-Link trägt den vollen Zustand: %.600s", body)
	}
	if !strings.Contains(body, "Spec 09") {
		t.Errorf("Suche ist ungedeckelt — Spec 09 fehlt")
	}
	if strings.Contains(body, "Plan Eins") || strings.Contains(body, "Dashboard README") {
		t.Errorf("die Suche filtert alle Gruppen")
	}
	if !strings.Contains(body, "10 von 13 Karten") {
		t.Errorf("die Trefferzeile fehlt: %.600s", body)
	}

	// Dieselbe Route ohne htmx liefert die ganze Seite.
	rec2 := c.do(t, "GET", "/nodes/vor/wissen", nil)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "<html") {
		t.Errorf("eine Browser-Navigation auf die Fragment-Route bekommt die Seite")
	}
}

func TestNodeWissen_TypeFilterHitsOnlyItsGroup(t *testing.T) {
	c := newCockpitTestServer(t)
	seedWissenTree(t, c)

	rec := c.do(t, "GET", "/nodes/vor?tab=wissen&typ=spec&in=sf", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Spec 09") {
		t.Errorf("ein Typ-Filter zeigt alle Treffer der Gruppe")
	}
	if strings.Contains(body, "Plan Eins") {
		t.Errorf("der Typ-Filter blendet andere Typen der Gruppe aus")
	}
	if !strings.Contains(body, "Dashboard README") {
		t.Errorf("der Typ-Filter lässt andere Gruppen in Ruhe")
	}
	if !strings.Contains(body, `class="wtype on"`) || !strings.Contains(body, "Filter zurücksetzen") {
		t.Errorf("der aktive Zähler ist markiert, der Filter lässt sich zurücksetzen")
	}
}

func TestNodeWissen_RepoIsFlat(t *testing.T) {
	c := newCockpitTestServer(t)
	seedWissenTree(t, c)

	rec := c.do(t, "GET", "/nodes/sf?tab=wissen", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "direkt hier") || strings.Contains(body, `href="/nodes/dash"`) {
		t.Errorf("ein Repo hat eine Herkunft — keine Gruppenköpfe, keine Geschwister")
	}
	for _, want := range []string{`<span class="tnum">10</span> Spec`, `href="/nodes/sf?tab=wissen&amp;typ=spec"`, "12 Karten", "Fallen der Linie"} {
		if !strings.Contains(body, want) {
			t.Errorf("Repo-Überblick ohne %q", want)
		}
	}
	if strings.Contains(body, "in=sf") {
		t.Errorf("am Repo filtert der Zähler ohne Gruppe")
	}
}

func TestNodeWissen_UnknownNodeIs404(t *testing.T) {
	c := newCockpitTestServer(t)
	if rec := c.do(t, "GET", "/nodes/gibtsnicht?tab=wissen", nil); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	if rec := c.do(t, "GET", "/nodes/gibtsnicht/wissen", nil); rec.Code != http.StatusNotFound {
		t.Errorf("fragment status %d, want 404", rec.Code)
	}
}
