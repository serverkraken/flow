package httpserver_test

// GET /nodes/{id}/artifacts is a NAVIGABLE page, not an htmx fragment.
// It used to answer with the bare #cockpit-artifacts fragment — no <html>,
// no app.css, no shell — so opening it in a browser rendered every artifact
// image at its natural size on a white background, with the rename/delete
// dialogs inlined. That is the "Artefakte werden nicht korrekt angezeigt"
// Soenne reported (2026-08-20). The Bibliothek already splits this correctly
// (/wissen/artefakte page vs /ui/wissen/artefakte fragment); the node gallery
// must follow the same rule.

import (
	"net/http"
	"strings"
	"testing"
)

func TestNodeArtifactsPage_IsAFullDocument(t *testing.T) {
	ts, cookie, ns, _, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)

	code, body := getN(t, ts, cookie, "/nodes/n1/artifacts")
	if code != http.StatusOK {
		t.Fatalf("GET artifacts = %d; body=%.300s", code, body)
	}
	for _, want := range []string{"<!doctype html>", "/static/app.css", "<body"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			t.Errorf("artifacts page must be a full document, missing %q; body=%.400s", want, body)
		}
	}
}

func TestNodeArtifactsFragment_StaysAFragmentForHtmx(t *testing.T) {
	ts, cookie, ns, _, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	req.Header.Set("HX-Request", "true")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	buf := make([]byte, 4096)
	n, _ := res.Body.Read(buf)
	body := string(buf[:n])
	if strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Errorf("htmx swap target must stay a fragment, got a full document: %.300s", body)
	}
}
