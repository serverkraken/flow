package httpserver_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// Screen 23 „Beim Anlegen": README als Kopf der Registerseite und die
// Zeitmessung sofort auf dem neuen Register.
func TestWebNodeCreate_OnCreateHooks(t *testing.T) {
	c := newCockpitTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"name": "Kalender Umbau", "kind": "engagement", "status": "active", "createReadme": "1", "startTimer": "1"} {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", "/nodes", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	cookieVal, _ := c.codec.Issue("u1")
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		body := rec.Body.String()
		if i := strings.Index(body, "alert-err"); i >= 0 {
			body = body[i : i+min(300, len(body)-i)]
		}
		t.Fatalf("status %d body=%.600s", rec.Code, body)
	}
	nodeID := strings.TrimPrefix(rec.Header().Get("Location"), "/nodes/")
	docs, _ := c.ds.List(context.Background(), "u1", &nodeID)
	if len(docs) != 1 || docs[0].Path != "readme" || docs[0].Title != "Kalender Umbau" {
		t.Errorf("README fehlt: %+v", docs)
	}
	running, ok, _ := c.ss.Running(context.Background(), "u1")
	if !ok || running.NodeID == nil || *running.NodeID != nodeID {
		t.Errorf("die Uhr läuft nicht auf dem neuen Register: %+v", running)
	}

	// Das Formular selbst: Abschnitte, Vorschau-Anker, Haken.
	form := c.do(t, "GET", "/nodes/new", nil).Body.String()
	for _, want := range []string{"Neues Register anlegen", `data-node-form`, `data-slug-preview`, `data-mono-preview`, `name="createReadme"`, `name="startTimer"`, "Wird angelegt als", `data-desc-vorhaben`} {
		if !strings.Contains(form, want) {
			t.Errorf("Formular ohne %q", want)
		}
	}
}

// Screen 29: ohne ein einziges Register ist der Kasten leer — und sagt, was
// zuerst kommt.
func TestWebNodesHome_FirstStart(t *testing.T) {
	c := newCockpitTestServer(t)
	body := c.do(t, "GET", "/nodes", nil).Body.String()
	for _, want := range []string{`data-first-start`, "Der Kasten ist leer", `href="/nodes/new?kind=engagement"`, `href="/einstellungen#sollzeiten"`} {
		if !strings.Contains(body, want) {
			t.Errorf("leerer Kasten ohne %q", want)
		}
	}
	c.seedNode(t, domain.Node{ID: "e1", OwnerID: "u1", Slug: "privat", Name: "Privat", Kind: domain.KindEngagement})
	if strings.Contains(c.do(t, "GET", "/nodes", nil).Body.String(), `data-first-start`) {
		t.Errorf("mit Register kein erster Start")
	}
}
