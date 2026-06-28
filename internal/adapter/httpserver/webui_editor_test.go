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

func TestEditorUpdateAndDeleteRedirect(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "doc-1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/edit",
		Title: "Edit Me", Body: "body",
	})

	updateForm := url.Values{"title": {"Updated"}, "body": {"new body"}}
	updateReq := authedEditorRequest(http.MethodPost, "/wissen/doc-1", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.SetPathValue("id", "doc-1")
	updateRec := httptest.NewRecorder()
	srv.handleWebEditorUpdate(updateRec, updateReq)
	if updateRec.Code != http.StatusSeeOther || updateRec.Header().Get("Location") != "/wissen/doc-1" {
		t.Fatalf("update code=%d location=%q", updateRec.Code, updateRec.Header().Get("Location"))
	}

	deleteReq := authedEditorRequest(http.MethodPost, "/wissen/doc-1/delete", nil)
	deleteReq.SetPathValue("id", "doc-1")
	deleteRec := httptest.NewRecorder()
	srv.handleWebEditorDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusSeeOther || deleteRec.Header().Get("Location") != "/wissen/frei" {
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

func authedEditorRequest(method, target string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
	}
	return req.WithContext(context.WithValue(req.Context(), userKey, domain.User{ID: "u1", Username: "msoent"}))
}
