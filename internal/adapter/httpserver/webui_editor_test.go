package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestEditorPreview(t *testing.T) {
	srv, _, _, _ := newWebWissenServer(t)
	body := "body=" + url.QueryEscape("# Title\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	req := httptest.NewRequest(http.MethodPost, "/wissen/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<table") {
		t.Fatalf("preview did not render markdown: %s", rec.Body.String())
	}
}

func TestEditorCreatePublishesEvent(t *testing.T) {
	srv, _, _, _ := newWebWissenServer(t)
	ch, cancel := srv.Bus.Subscribe("u1")
	defer cancel()

	form := url.Values{
		"type":  {"free"},
		"path":  {"notes/new"},
		"title": {"New Note"},
		"body":  {"hello"},
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/wissen/id-1" {
		t.Fatalf("Location=%q", loc)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentCreated {
			t.Fatalf("event=%q", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no document.created event published")
	}
}

func TestEditorNewAndEditRender(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit",
		Title: "Edit Me", Body: "body",
	})

	newReq := authedEditorRequest(http.MethodGet, "/wissen/neu", nil)
	newRec := httptest.NewRecorder()
	srv.handleWebEditorNew(newRec, newReq)
	if newRec.Code != http.StatusOK || !strings.Contains(newRec.Body.String(), `action="/wissen"`) {
		t.Fatalf("new editor code=%d body=%.400s", newRec.Code, newRec.Body.String())
	}

	editReq := authedEditorRequest(http.MethodGet, "/wissen/doc-1/bearbeiten", nil)
	editReq.SetPathValue("id", "doc-1")
	editRec := httptest.NewRecorder()
	srv.handleWebEditorEdit(editRec, editReq)
	if editRec.Code != http.StatusOK || !strings.Contains(editRec.Body.String(), `action="/wissen/doc-1"`) {
		t.Fatalf("edit editor code=%d body=%.400s", editRec.Code, editRec.Body.String())
	}
}

func TestEditorNewUsesShelfAndCockpitPresets(t *testing.T) {
	srv, _, _, projects := newWebWissenServer(t)
	ctx := context.Background()
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatal(err)
	}

	req := authedEditorRequest(http.MethodGet, "/wissen/neu?node=n1&type=project&path=readme", nil)
	rec := httptest.NewRecorder()
	srv.handleWebEditorNew(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.500s", rec.Code, body)
	}
	for _, want := range []string{
		`name="type"`, `value="project" selected`, `name="projectId"`, `value="n1" selected`,
		`name="path" value="readme"`, `data-metadata-field="date"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("preset editor missing %q: %.1200s", want, body)
		}
	}
}

func TestEditorEditExposesCanonicalMetadataFields(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-07-03", Date: &day,
		Title: "Daily", Body: "body",
	})

	req := authedEditorRequest(http.MethodGet, "/wissen/doc-1/bearbeiten", nil)
	req.SetPathValue("id", "doc-1")
	rec := httptest.NewRecorder()
	srv.handleWebEditorEdit(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.500s", rec.Code, body)
	}
	for _, want := range []string{`name="type"`, `name="date" value="2026-07-03"`, `data-derived-path`} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit metadata missing %q: %.1200s", want, body)
		}
	}
	if strings.Contains(body, `value="daily" disabled`) {
		t.Fatalf("document type must be editable: %.800s", body)
	}
}

func TestEditorUpdateAndDeleteRedirect(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit",
		Title: "Edit Me", Body: "body",
	})

	updateForm := url.Values{
		"type": {"daily"}, "date": {"2026-07-03"},
		"title": {"Updated"}, "body": {"new body"},
	}
	updateReq := authedEditorRequest(http.MethodPost, "/wissen/doc-1", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.SetPathValue("id", "doc-1")
	updateRec := httptest.NewRecorder()
	srv.handleWebEditorUpdate(updateRec, updateReq)
	if updateRec.Code != http.StatusSeeOther || updateRec.Header().Get("Location") != "/wissen/doc-1" {
		t.Fatalf("update code=%d location=%q", updateRec.Code, updateRec.Header().Get("Location"))
	}
	moved, err := docs.Get(ctx, "u1", "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Type != domain.DocDaily || moved.Path != "daily/2026-07-03" || moved.Date == nil {
		t.Fatalf("editor did not reclassify metadata: %+v", moved)
	}

	deleteReq := authedEditorRequest(http.MethodPost, "/wissen/doc-1/delete", nil)
	deleteReq.SetPathValue("id", "doc-1")
	deleteRec := httptest.NewRecorder()
	srv.handleWebEditorDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusSeeOther || deleteRec.Header().Get("Location") != "/wissen/typ?type=daily" {
		t.Fatalf("delete code=%d location=%q", deleteRec.Code, deleteRec.Header().Get("Location"))
	}
}

func TestWebEditorCreate_ParsesTags(t *testing.T) {
	t.Parallel()
	srv, _, docs, _ := newWebWissenServer(t)

	form := url.Values{
		"type":  {"free"},
		"path":  {"e1"},
		"title": {"T"},
		"body":  {"b"},
		"tags":  {"go tui"},
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther, got %d body=%s", rec.Code, rec.Body.String())
	}

	doc, err := docs.Get(context.Background(), "u1", "id-1")
	if err != nil {
		t.Fatalf("Get doc: %v", err)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "go" || doc.Tags[1] != "tui" {
		t.Fatalf("expected tags [go tui], got %v", doc.Tags)
	}
}

// TestEditorEditModeHasDeleteConfirmDialog covers the Task 5 fix (post-review):
// Delete moved off the Lesesaal document view page to the edit page, edit
// mode only — matching the Mockup (Z.688–695: the document page shows only
// Bearbeiten + Anpinnen) and the same L2 doctrine already applied to nodes
// ("Move/Status/Delete auf der Edit-Seite"). The trigger + ConfirmDialog use
// the shared component (no native browser confirm/alert popup); the new-doc
// form must not show a delete affordance at all (nothing to delete yet).
func TestEditorEditModeHasDeleteConfirmDialog(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit",
		Title: "Edit Me", Body: "body",
	})

	editReq := authedEditorRequest(http.MethodGet, "/wissen/doc-1/bearbeiten", nil)
	editReq.SetPathValue("id", "doc-1")
	editRec := httptest.NewRecorder()
	srv.handleWebEditorEdit(editRec, editReq)
	editBody := editRec.Body.String()
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit editor code=%d body=%.400s", editRec.Code, editBody)
	}
	for _, want := range []string{
		`data-dialog-open="del-doc-1"`,
		`<dialog id="del-doc-1"`,
		`hx-post="/wissen/doc-1/delete"`,
	} {
		if !strings.Contains(editBody, want) {
			t.Fatalf("edit editor missing delete affordance %q: %.800s", want, editBody)
		}
	}

	newReq := authedEditorRequest(http.MethodGet, "/wissen/neu", nil)
	newRec := httptest.NewRecorder()
	srv.handleWebEditorNew(newRec, newReq)
	newBody := newRec.Body.String()
	if newRec.Code != http.StatusOK {
		t.Fatalf("new editor code=%d body=%.400s", newRec.Code, newBody)
	}
	if strings.Contains(newBody, `data-dialog-open="del-`) || strings.Contains(newBody, "<dialog id=\"del-") {
		t.Fatalf("new-doc editor must not show a delete affordance (nothing to delete yet): %.400s", newBody)
	}
}

// TestEditorLesesaalPanelAndField verifies the editor form + preview run on
// the Lesesaal `.panel` primitive (no leftover Kristall glass/shadow chrome)
// and the shared `.field` input class, per L3 Task 8: form + preview
// containers carry `panel` (not `glass`/`bg-surface`), every
// text/select/textarea input carries `field`, and the form still posts to
// vm.Action() with all fields + the save button preserved.
func TestEditorLesesaalPanelAndField(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit",
		Title: "Edit Me", Body: "body",
	})

	// New-doc form.
	newReq := authedEditorRequest(http.MethodGet, "/wissen/neu", nil)
	newRec := httptest.NewRecorder()
	srv.handleWebEditorNew(newRec, newReq)
	newBody := newRec.Body.String()
	if newRec.Code != http.StatusOK {
		t.Fatalf("new editor code=%d body=%.400s", newRec.Code, newBody)
	}
	if !strings.Contains(newBody, `action="/wissen"`) {
		t.Fatalf("new editor form must still post to vm.Action(): %.400s", newBody)
	}
	if editorPanel := scopeToEditorPanel(t, newBody); !strings.Contains(editorPanel, "panel") {
		t.Fatalf("new editor form/preview panel must carry the Lesesaal .panel class: %.800s", editorPanel)
	}
	if editorPanel := scopeToEditorPanel(t, newBody); strings.Contains(editorPanel, "glass") || strings.Contains(editorPanel, "bg-surface") || strings.Contains(editorPanel, "shadow-soft") {
		t.Fatalf("new editor form/preview panel must not use Kristall glass/shadow-soft/bg-surface anymore: %.800s", editorPanel)
	}
	for _, name := range []string{`name="type"`, `name="projectId"`, `name="path"`, `name="title"`, `name="body"`} {
		if !strings.Contains(newBody, name) {
			t.Fatalf("new editor must preserve field %s: %.800s", name, newBody)
		}
	}
	if strings.Count(newBody, "field") < 5 {
		t.Fatalf("expected the .field class on the new-doc inputs, got body=%.800s", newBody)
	}
	if !strings.Contains(newBody, `type="submit"`) {
		t.Fatalf("save button must be preserved: %.400s", newBody)
	}

	// Edit-doc form (type/path are now intentionally editable through the
	// transactional MoveDocument use case).
	editReq := authedEditorRequest(http.MethodGet, "/wissen/doc-1/bearbeiten", nil)
	editReq.SetPathValue("id", "doc-1")
	editRec := httptest.NewRecorder()
	srv.handleWebEditorEdit(editRec, editReq)
	editBody := editRec.Body.String()
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit editor code=%d body=%.400s", editRec.Code, editBody)
	}
	if !strings.Contains(editBody, `action="/wissen/doc-1"`) {
		t.Fatalf("edit editor form must still post to vm.Action(): %.400s", editBody)
	}
	if editorPanel := scopeToEditorPanel(t, editBody); !strings.Contains(editorPanel, "panel") {
		t.Fatalf("edit editor form/preview panel must carry the Lesesaal .panel class: %.800s", editorPanel)
	}
	if editorPanel := scopeToEditorPanel(t, editBody); strings.Contains(editorPanel, "glass") || strings.Contains(editorPanel, "bg-surface") || strings.Contains(editorPanel, "shadow-soft") {
		t.Fatalf("edit editor form/preview panel must not use Kristall glass/shadow-soft/bg-surface anymore: %.800s", editorPanel)
	}
	if !strings.Contains(editBody, "field") {
		t.Fatalf("edit editor metadata fields must carry the .field class: %.800s", editBody)
	}
	if !strings.Contains(editBody, `name="type"`) || !strings.Contains(editBody, `name="path"`) {
		t.Fatalf("edit editor must expose type/path through MoveDocument: %.800s", editBody)
	}
}

// scopeToEditorPanel narrows a full editor-page render down to the
// form+preview panel (the two containers K4 Task 5 swaps to `glass`), so
// assertions don't trip on the unrelated `bg-surface` used by AppShell's
// mobile nav elsewhere on the page.
func scopeToEditorPanel(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, "<form action=")
	end := strings.LastIndex(page, "</section>")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("could not locate editor form/preview panel in page: %.800s", page)
	}
	return page[start : end+len("</section>")]
}

func authedEditorRequest(method, target string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
	}
	return req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
}

// TestEditorPreviewResolvesArtifactEmbedWithNode is the Task 6 / Spec §13
// mandatory test: POST /wissen/preview with an artifact-bearing node id in
// the "node" form field must resolve a ![[slug]] embed to the actual figure
// (the real serve-route <img src>), not the unresolved chip — proving the
// preview handler's node-aware resolver (buildEditorArtifactResolver) is
// actually wired in, not just present as dead code.
func TestEditorPreviewResolvesArtifactEmbedWithNode(t *testing.T) {
	srv, _, _, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "bild", Name: "bild.png", Mime: "image/png", Ref: "abcdef123456",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	form := url.Values{"body": {"![[bild]]"}, "node": {"n1"}}
	req := httptest.NewRequest(http.MethodPost, "/wissen/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`class="figure"`, `src="/nodes/n1/artifacts/bild?v=abcdef123456"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q — preview did not resolve the artifact embed via node context: %.800s", want, body)
		}
	}
}

// TestEditorPreviewResolvesFreeArtifactEmbed is the free-artifacts Task 3
// mandatory Spec §13 test: POST /wissen/preview with body="![[bild]]" and
// node="" (a free image "bild" exists) must resolve it to the real
// /artefakte/{slug} figure — proving buildEditorArtifactResolver's free
// branch (nodeID=="" → ListFree via ListArtifacts) is actually wired in.
func TestEditorPreviewResolvesFreeArtifactEmbed(t *testing.T) {
	srv, _, _, _ := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "bild", Name: "bild.png", Mime: "image/png", Ref: "abcdef123456",
	}); err != nil {
		t.Fatalf("seed free artifact: %v", err)
	}

	form := url.Values{"body": {"![[bild]]"}, "node": {""}}
	req := httptest.NewRequest(http.MethodPost, "/wissen/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`class="figure"`, `src="/artefakte/bild?v=abcdef123456"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q — preview did not resolve the free artifact embed: %.800s", want, body)
		}
	}
}

// TestEditorPreviewWithoutNodeLeavesEmbedUnresolved covers the counterpart
// state: no "node" (and no "projectId") field at all — the same body renders
// the unresolved wikilink-broken chip instead of a figure, exactly as before
// Task 6 (a doc/selection without a node never had artifacts to resolve
// against).
func TestEditorPreviewWithoutNodeLeavesEmbedUnresolved(t *testing.T) {
	srv, _, _, _ := newWebWissenServer(t)

	form := url.Values{"body": {"![[bild]]"}}
	req := httptest.NewRequest(http.MethodPost, "/wissen/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "wikilink-broken") {
		t.Fatalf("expected the unresolved chip without a node: %.800s", body)
	}
	if strings.Contains(body, `class="figure"`) {
		t.Fatalf("must not render a figure without a node: %.800s", body)
	}
}

// TestEditorToolbarHasInsertPickerAffordances asserts the Task 6 static
// surface the brief calls out as the non-JS-testable part's Go-side proxy:
// both toolbar buttons, the editor-insert.js asset tag, and — in edit mode —
// the hidden "node" field the preview POST/picker GETs rely on.
func TestEditorToolbarHasInsertPickerAffordances(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	nodeID := "n1"
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit", NodeID: &nodeID,
		Title: "Edit Me", Body: "body",
	})

	editReq := authedEditorRequest(http.MethodGet, "/wissen/doc-1/bearbeiten", nil)
	editReq.SetPathValue("id", "doc-1")
	editRec := httptest.NewRecorder()
	srv.handleWebEditorEdit(editRec, editReq)
	editBody := editRec.Body.String()
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit editor code=%d body=%.400s", editRec.Code, editBody)
	}
	for _, want := range []string{
		`data-insert-toggle="insert-picker-artefakte"`,
		`data-insert-toggle="insert-picker-seiten"`,
		`src="/static/js/editor-insert.js`,
		`<input type="hidden" name="node" value="n1"`,
		`id="editor-body"`,
		// The Artefakt-picker fires GET requests; htmx 2.x does NOT auto-include
		// the enclosing form on GET (only on POST), so both the toggle button and
		// the filter input must hx-include the node/projectId fields — otherwise
		// the picker's ListArtifacts call gets an empty node and shows nothing.
		// Include order [name=node], [name=projectId] is deliberately distinct
		// from the preview textarea's [name=projectId], [name=node] so this
		// substring identifies only the picker elements.
		`hx-get="/ui/editor/artefakte" hx-include="[name=node], [name=projectId]"`,
		`hx-include="[name=node], [name=projectId]" hx-params="projectId,node,aq"`,
	} {
		if !strings.Contains(editBody, want) {
			t.Fatalf("edit editor missing %q: %.1200s", want, editBody)
		}
	}
	// The Seiten-picker needs no node context — it must NOT carry the picker
	// node-include (it lists all owner docs regardless).
	if strings.Contains(editBody, `hx-get="/ui/editor/seiten" hx-include="[name=node]`) {
		t.Fatalf("seiten picker must not include node context")
	}
}

// TestEditorCreate_InvalidType verifies that renderEditorError is called
// when CreateDocument returns ErrInvalidDocument (bad type field).
func TestEditorCreate_InvalidType(t *testing.T) {
	srv, _, _, _ := newWebWissenServer(t)

	form := url.Values{
		"type":  {"notavalidtype"},
		"path":  {"notes/new"},
		"title": {"T"},
		"body":  {"b"},
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
	rec := httptest.NewRecorder()

	srv.handleWebEditorCreate(rec, req)

	// renderEditorError sets the status before rendering the editor page.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%.300s", rec.Code, rec.Body.String())
	}
}
