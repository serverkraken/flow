package apiclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func stubDoc(id, title string) domain.Document {
	return domain.Document{
		ID:        id,
		Type:      domain.DocFree,
		Path:      "notes/test",
		Title:     title,
		Body:      "hello",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestCreateDocument_PostsBody(t *testing.T) {
	want := stubDoc("doc-1", "My Note")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/documents" {
			t.Errorf("path: got %s, want /api/v1/documents", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization: got %q, want Bearer tok", r.Header.Get("Authorization"))
		}
		var body apiclient.CreateDocumentInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Title != "My Note" {
			t.Errorf("title: got %q, want %q", body.Title, "My Note")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.CreateDocument(context.Background(), apiclient.CreateDocumentInput{
		Type:  "free",
		Path:  "notes/test",
		Title: "My Note",
		Body:  "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("id: got %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("title: got %q, want %q", got.Title, want.Title)
	}
}

func TestListDocuments_DecodesSlice(t *testing.T) {
	docs := []domain.Document{
		stubDoc("doc-1", "First"),
		stubDoc("doc-2", "Second"),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/documents" {
			t.Errorf("path: got %s, want /api/v1/documents", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.ListDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
	if got[0].ID != "doc-1" {
		t.Errorf("got[0].ID: got %q, want doc-1", got[0].ID)
	}
	if got[1].Title != "Second" {
		t.Errorf("got[1].Title: got %q, want Second", got[1].Title)
	}
}

func TestGetDocument_DecodesDoc(t *testing.T) {
	want := stubDoc("doc-42", "Specific Note")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/doc-42" {
			t.Errorf("path: got %s, want /api/v1/documents/doc-42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.GetDocument(context.Background(), "doc-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "doc-42" {
		t.Errorf("id: got %q, want doc-42", got.ID)
	}
	if got.Title != "Specific Note" {
		t.Errorf("title: got %q, want Specific Note", got.Title)
	}
}

func TestUpdateDocument_PutsBody(t *testing.T) {
	want := stubDoc("doc-5", "Updated")
	want.Title = "Updated"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method: got %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/doc-5" {
			t.Errorf("path: got %s, want /api/v1/documents/doc-5", r.URL.Path)
		}
		var body apiclient.UpdateDocumentInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Title != "Updated" {
			t.Errorf("title: got %q, want Updated", body.Title)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.UpdateDocument(context.Background(), "doc-5", apiclient.UpdateDocumentInput{
		Title: "Updated",
		Body:  "new body",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Updated" {
		t.Errorf("title: got %q, want Updated", got.Title)
	}
}

func TestPatchDocument_SendsOnlySuppliedFieldsAndCAS(t *testing.T) {
	expected := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	title := "Updated"
	want := stubDoc("doc-patch", title)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/documents/doc-patch" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["body"]; ok {
			t.Fatal("omitted body was serialized")
		}
		if _, ok := raw["tags"]; ok {
			t.Fatal("omitted tags were serialized")
		}
		if _, ok := raw["title"]; !ok {
			t.Fatal("title missing from patch")
		}
		if _, ok := raw["expectedUpdatedAt"]; !ok {
			t.Fatal("expectedUpdatedAt missing from patch")
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.PatchDocument(context.Background(), "doc-patch", apiclient.PatchDocumentInput{
		Title: &title, ExpectedUpdatedAt: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != title {
		t.Fatalf("title = %q, want %q", got.Title, title)
	}
}

func TestMoveDocument_PostsCompleteMetadata(t *testing.T) {
	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/documents/doc-5/move" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var in apiclient.MoveDocumentInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if in.Type != "daily" || in.Date == nil || !in.Date.Equal(day) {
			t.Fatalf("input = %+v", in)
		}
		_ = json.NewEncoder(w).Encode(domain.Document{ID: "doc-5", Type: domain.DocDaily, Path: "daily/2026-07-03", Date: &day})
	}))
	defer srv.Close()

	got, err := apiclient.New(srv.URL, "tok").MoveDocument(context.Background(), "doc-5", apiclient.MoveDocumentInput{
		Type: "daily", Date: &day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "daily/2026-07-03" {
		t.Fatalf("result = %+v", got)
	}
}

func TestDeleteDocument_Issues204(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization: got %q, want Bearer tok", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	err := c.DeleteDocument(context.Background(), "doc-99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %s, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/documents/doc-99" {
		t.Errorf("path: got %s, want /api/v1/documents/doc-99", gotPath)
	}
}

func TestCreateDocument_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	_, err := c.CreateDocument(context.Background(), apiclient.CreateDocumentInput{
		Type:  "free",
		Path:  "notes/test",
		Title: "Bad",
	})
	if err == nil {
		t.Fatal("expected error for 422, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected 422 in error, got: %v", err)
	}
}

func TestListDocuments_TagQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	if _, err := c.ListDocuments(context.Background(), "go", "tui"); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "tag=go&tag=tui" {
		t.Fatalf("query = %q, want tag=go&tag=tui", gotQuery)
	}
}

func TestClientTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/tags" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag":"go","count":2}]`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	got, err := c.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "go" || got[0].Count != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestSearch_QueryAndTags(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"a","snippet":"hi"}]`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	hits, err := c.Search(context.Background(), "kompend", "go")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "q=kompend&tag=go" {
		t.Fatalf("query = %q, want q=kompend&tag=go", gotQuery)
	}
	if len(hits) != 1 || hits[0].ID != "a" || hits[0].Snippet != "hi" {
		t.Fatalf("decode failed: %#v", hits)
	}
}

func TestClient_Backlinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/d1/backlinks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"s1","path":"src","title":"Src","type":"free"}]`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	refs, err := c.Backlinks(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "s1" || refs[0].Type != domain.DocFree {
		t.Fatalf("refs = %v", refs)
	}
}

func TestImportDocument_PostsBodyAndPath(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.Document{ID: "d1"})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	d0 := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	pid := "p1"
	_, err := c.ImportDocument(context.Background(), apiclient.ImportDocumentInput{
		Type: "project", Path: "projects/foo/readme", Title: "Foo", Body: "B", Date: &d0, NodeID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/documents/import" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"projectId":"p1"`) || !strings.Contains(gotBody, `"date":"2026-04-28`) {
		t.Fatalf("body missing fields: %s", gotBody)
	}
}

func TestImportDocument_ConflictIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	_, err := c.ImportDocument(context.Background(), apiclient.ImportDocumentInput{Type: "free", Path: "x", Title: "T", Body: "B"})
	if !apiclient.IsConflict(err) {
		t.Fatalf("want IsConflict, got %v", err)
	}
}

func TestListDocumentsScoped_QueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	pid := "proj-a"
	if _, err := c.ListDocumentsScoped(context.Background(), &pid, "go"); err != nil {
		t.Fatalf("ListDocumentsScoped: %v", err)
	}
	if !strings.Contains(gotQuery, "projectId=proj-a") || !strings.Contains(gotQuery, "tag=go") {
		t.Fatalf("query = %q, want projectId=proj-a & tag=go", gotQuery)
	}

	if _, err := c.SearchScoped(context.Background(), "widget", strptrAC("none")); err != nil {
		t.Fatalf("SearchScoped: %v", err)
	}
	if !strings.Contains(gotQuery, "projectId=none") || !strings.Contains(gotQuery, "q=widget") {
		t.Fatalf("query = %q, want projectId=none & q=widget", gotQuery)
	}
}

func strptrAC(s string) *string { return &s }
