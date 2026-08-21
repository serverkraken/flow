package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Unbekannte Adressen bekommen die Fehlerseite in der Hülle — nicht Gos
// Klartext auf Schwarz (Ist-Befund #14).
func TestWebNotFound_UnknownPathGetsTheErrorPage(t *testing.T) {
	c := newCockpitTestServer(t)
	rec := c.do(t, "GET", "/gibtsnicht", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<!doctype html>", `data-error-page="404"`, "Diese Seite gibt es nicht", `id="app-rail"`, "/gibtsnicht"} {
		if !strings.Contains(body, want) {
			t.Errorf("404-Seite ohne %q: %.500s", want, body)
		}
	}
	// Ein Handler, der selbst 404 antwortet, nutzt dieselbe Seite.
	rec = c.do(t, "GET", "/nodes/gibtsnicht", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Dieses Register gibt es nicht") {
		t.Errorf("Register-404: %d %.300s", rec.Code, rec.Body.String())
	}
}

// Ein htmx-Aufruf bekommt die Fläche allein — eine ganze Seite in einem
// Fragment-Ziel wäre die Hülle in der Hülle.
func TestWebNotFound_HtmxGetsTheBodyOnly(t *testing.T) {
	c := newCockpitTestServer(t)
	req, _ := http.NewRequest("GET", "/nodes/gibtsnicht/wissen", nil)
	req.Header.Set("HX-Request", "true")
	cookieVal, _ := c.codec.Issue("u1")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound || strings.Contains(body, "<html") || !strings.Contains(body, `data-error-page="404"`) {
		t.Errorf("htmx-404: %d %.300s", rec.Code, body)
	}
}

// Die API bleibt Klartext; ohne Sitzung gibt es die Seite ohne Schiene —
// und keine Umleitung, die verriete, welche Adressen es gibt.
func TestWebNotFound_APIStaysPlainAndAnonymousGetsNoShell(t *testing.T) {
	c := newCockpitTestServer(t)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gibtsnicht", nil)
	c.srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("API-404 muss Klartext bleiben: %d %.200s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/gibtsnicht", nil)
	c.srv.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound || !strings.Contains(body, `data-error-page="404"`) || strings.Contains(body, `id="app-rail"`) || strings.Contains(body, "sse-connect") {
		t.Errorf("anonym: 404 ohne Schiene und ohne SSE erwartet: %d %.300s", rec.Code, body)
	}
}
