package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

// TestEditorResolve_AnswersLikeTheReadingView pins the editor's resolution
// endpoint to the reading view's rules: a [[path]] resolves within the
// editor's node scope to the document's href + title, a ![[slug]] to the
// real serve route with the cache-busting ?v=ref, and anything unknown is
// simply absent (the editor renders it broken, as RenderDocument would).
func TestEditorResolve_AnswersLikeTheReadingView(t *testing.T) {
	srv, _, docs, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	n1 := "n1"
	if _, err := docs.Create(ctx, domain.Document{ID: "d-spec", OwnerID: "u1", NodeID: &n1, Type: domain.DocSpec, Path: "specs/plan", Title: "Der Plan", Body: "x"}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{OwnerID: "u1", NodeID: "n1", Slug: "bild", Name: "bild.png", Mime: "image/png", Ref: "abcdef123456", Width: 640, Height: 480}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/editor/aufloesen?node=n1&wl=specs/plan&wl=gibt/es/nicht&embed=bild&embed=fehlt", nil)
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()
	srv.handleWebEditorResolve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var got editorResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, rec.Body.String())
	}
	if wl, ok := got.Wikilinks["specs/plan"]; !ok || wl.Href != "/wissen/d-spec" || wl.Title != "Der Plan" {
		t.Errorf("wikilink specs/plan = %+v (ok=%v), want /wissen/d-spec · Der Plan", wl, ok)
	}
	if _, ok := got.Wikilinks["gibt/es/nicht"]; ok {
		t.Errorf("unknown wikilink must be absent, got %+v", got.Wikilinks)
	}
	if em, ok := got.Embeds["bild"]; !ok || em.Src != "/nodes/n1/artifacts/bild?v=abcdef123456" || !em.IsImage || em.Width != 640 || em.Name != "bild.png" {
		t.Errorf("embed bild = %+v (ok=%v), want the serve route with ?v=ref, image 640 wide", em, ok)
	}
	if _, ok := got.Embeds["fehlt"]; ok {
		t.Errorf("unknown embed must be absent, got %+v", got.Embeds)
	}
}

// TestEditorResolve_ForeignTenantSeesNothing: a second owner asking about the
// first owner's targets gets empty maps — ListDocuments/ListArtifacts are
// owner-scoped, and the node chain of a foreign node resolves to nothing.
func TestEditorResolve_ForeignTenantSeesNothing(t *testing.T) {
	srv, _, docs, projects := newWebWissenServer(t)
	ctx := context.Background()
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if _, err := docs.Create(ctx, domain.Document{ID: "d-free", OwnerID: "u1", Type: domain.DocFree, Path: "notiz", Title: "Notiz", Body: "x"}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ui/editor/aufloesen?node=n1&wl=notiz&embed=bild", nil)
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u2", Username: "other"}))
	rec := httptest.NewRecorder()
	srv.handleWebEditorResolve(rec, req)
	var got editorResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, rec.Body.String())
	}
	if len(got.Wikilinks) != 0 || len(got.Embeds) != 0 {
		t.Errorf("foreign tenant resolved something: %+v", got)
	}
}
