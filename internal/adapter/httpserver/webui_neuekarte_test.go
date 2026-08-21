package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// ⌘N: das Formular kommt mit dem Register, auf dem man steht; Anlegen
// leitet den Pfad ab und öffnet den Editor.
func TestNeueKarte_FormAndCreate(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Slug: "flow", Name: "flow", Kind: domain.KindRepo})

	rec := c.do(t, "GET", "/ui/wissen/neu?node=n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") || !strings.Contains(body, `value="n1" selected`) || !strings.Contains(body, `value="project" checked`) {
		t.Errorf("Formular: %.600s", body)
	}

	rec = c.do(t, "POST", "/wissen/schnell", map[string]string{"node": "n1", "type": "plan", "title": "Kalender+Umbau"})
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "/bearbeiten") {
		t.Fatalf("Anlegen: %d %s %.300s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	docs, _ := c.ds.List(context.Background(), "u1", nil)
	if len(docs) != 1 || docs[0].Path != "plans/2026-06-30-kalender-umbau" || docs[0].Title != "Kalender Umbau" || docs[0].Type != domain.DocPlan {
		t.Errorf("angelegt: %+v", docs)
	}

	// Ein belegter Pfad bekommt eine Nummer statt eines Fehlers.
	rec = c.do(t, "POST", "/wissen/schnell", map[string]string{"node": "n1", "type": "plan", "title": "Kalender+Umbau"})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("zweites Anlegen: %d %.300s", rec.Code, rec.Body.String())
	}
	docs, _ = c.ds.List(context.Background(), "u1", nil)
	if len(docs) != 2 {
		t.Fatalf("docs = %d", len(docs))
	}
	paths := docs[0].Path + " " + docs[1].Path
	if !strings.Contains(paths, "plans/2026-06-30-kalender-umbau-2") {
		t.Errorf("Nummer fehlt: %s", paths)
	}

	// Die Tagesnotiz von heute wird geöffnet, nicht verdoppelt.
	first := c.do(t, "POST", "/wissen/schnell", map[string]string{"type": "daily"})
	second := c.do(t, "POST", "/wissen/schnell", map[string]string{"type": "daily"})
	if first.Code != http.StatusSeeOther || second.Code != http.StatusSeeOther || first.Header().Get("Location") != second.Header().Get("Location") {
		t.Errorf("Tagesnotiz: %s vs %s", first.Header().Get("Location"), second.Header().Get("Location"))
	}

	// Ohne Titel kein Pfad — das Formular kommt mit Hinweis zurück.
	rec = c.do(t, "POST", "/wissen/schnell", map[string]string{"type": "project", "node": "n1"})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Ein Titel fehlt") {
		t.Errorf("ohne Titel: %d %.300s", rec.Code, rec.Body.String())
	}
}
