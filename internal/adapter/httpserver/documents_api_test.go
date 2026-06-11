// internal/adapter/httpserver/documents_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func newDocsAPIEnv(t *testing.T, sub string) (domain.User, chi.Router) {
	t.Helper()
	u, err := pgstore.NewUsers(pgTestStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountDocumentsAPI(r, DocumentsAPIDeps{Store: pgstore.NewDocuments(pgTestStore)})
	return u, r
}

func docReq(t *testing.T, r chi.Router, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDocumentsAPI_CRUDWithIfMatch(t *testing.T) {
	_, r := newDocsAPIEnv(t, "api-docs-1")

	// Create: If-Match: 0
	body := `{"body":"# Ideen"}`
	rec := docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", body, map[string]string{"If-Match": "0"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}

	// GET liefert body + version
	rec = docReq(t, r, "GET", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	if doc["body"] != "# Ideen" || doc["version"].(float64) != 1 {
		t.Fatalf("get payload: %v", doc)
	}

	// Update mit If-Match
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"v2"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	// Stale → 412
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"stale"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale: want 412, got %d", rec.Code)
	}
	// Ohne If-Match → 422
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"x"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("no if-match: want 422, got %d", rec.Code)
	}

	// Liste + Suche
	rec = docReq(t, r, "GET", "/documents?prefix=projects/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	// DELETE idempotent
	rec = docReq(t, r, "DELETE", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = docReq(t, r, "GET", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("after delete: want 404, got %d", rec.Code)
	}
}

func TestDocumentsAPI_RepoNoteAlias(t *testing.T) {
	_, r := newDocsAPIEnv(t, "api-docs-2")

	key := "git:github.com/serverkraken/flow"
	escaped := url.PathEscape(key)

	// PUT über den Alias legt das Dokument unter dem Konventions-Pfad an
	rec := docReq(t, r, "PUT", "/repos/"+escaped+"/note", `{"body":"repo wisdom"}`, map[string]string{"If-Match": "0"})
	if rec.Code != http.StatusOK {
		t.Fatalf("alias put: %d %s", rec.Code, rec.Body)
	}
	// GET über den Alias
	rec = docReq(t, r, "GET", "/repos/"+escaped+"/note", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("alias get: %d", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	if doc["repo_key"] != key {
		t.Errorf("repo_key roundtrip: %v", doc["repo_key"])
	}
	// und über den documents-Pfad (Spec: Lookup wahlweise)
	rec = docReq(t, r, "GET", "/documents/repos/"+url.PathEscape(escaped)+".md", "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("path lookup of repo note: %d", rec.Code)
	}
	// fehlender Key → 404
	rec = docReq(t, r, "GET", "/repos/"+url.PathEscape("git:github.com/x/y")+"/note", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing repo note: want 404, got %d", rec.Code)
	}
}
