package httpserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// Screen 30: Konto, Sollzeiten, Sprache, Zugänge, Daten — mit Kasten-Spalte
// und Kennzahlen. Kein Erscheinungsbild, solange es keinen Dunkel-Zwilling
// gibt.
func TestEinstellungen_Screen30(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Slug: "privat", Name: "Privat", Kind: domain.KindEngagement})
	c.seedNode(t, domain.Node{ID: "e2", OwnerID: "u1", Slug: "kunde", Name: "Kunde", Kind: domain.KindEngagement})

	rec := c.do(t, "GET", "/einstellungen", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="konto"`, `id="sollzeiten"`, `id="sprache"`, `id="zugaenge"`, `id="daten"`,
		"msoent", "Martin", "m@x", "verwaltet der Anmeldedienst",
		`id="einstellungen-target"`, "defaultTargetMin",
		`action="/ui/einstellungen/sprache"`, `value="de"`, `value="en"`,
		`href="/dayoffs"`, `href="/export"`,
		"2</span>", // zwei Engagements im Kasten
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Einstellungen ohne %q", want)
		}
	}
	if strings.Contains(body, "Erscheinungsbild") {
		t.Errorf("kein Erscheinungsbild ohne Dunkel-Zwilling")
	}
}

func TestEinstellungen_LanguageCookie(t *testing.T) {
	c := newCockpitTestServer(t)
	rec := c.do(t, "POST", "/ui/einstellungen/sprache", map[string]string{"lang": "en"})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	var set bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "flow_lang" && ck.Value == "en" && ck.MaxAge > 0 {
			set = true
		}
	}
	if !set {
		t.Errorf("flow_lang=en nicht gesetzt: %v", rec.Header()["Set-Cookie"])
	}
	rec = c.do(t, "POST", "/ui/einstellungen/sprache", map[string]string{"lang": ""})
	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "flow_lang" && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("Browservorgabe löscht das Cookie nicht: %v", rec.Header()["Set-Cookie"])
	}
}
