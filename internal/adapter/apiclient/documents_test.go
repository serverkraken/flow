package apiclient_test

import (
	"context"
	"encoding/json"
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
