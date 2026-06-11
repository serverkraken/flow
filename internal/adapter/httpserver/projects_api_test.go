package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func newProjectsAPIEnv(t *testing.T, sub string) chi.Router {
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
	MountProjectsAPI(r, ProjectsAPIDeps{Projects: pgstore.NewProjects(pgTestStore)})
	return r
}

func TestProjectsAPI_CreateListRenameArchive(t *testing.T) {
	r := newProjectsAPIEnv(t, "api-proj-1")
	do := func(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// Create
	rec := do("POST", "/projects", `{"name":"Mein Projekt","slug":"mein-projekt"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var proj map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &proj)
	id, _ := proj["id"].(string)
	if id == "" || proj["version"].(float64) != 1 {
		t.Fatalf("create payload: %v", proj)
	}
	// Create ohne Name → 422
	rec = do("POST", "/projects", `{"slug":"x"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("create w/o name: want 422, got %d", rec.Code)
	}

	// List (default: nur aktive)
	rec = do("GET", "/projects", "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("list: %d len=%d", rec.Code, len(page.Items))
	}

	// Rename via PUT mit If-Match
	rec = do("PUT", "/projects/"+id, `{"name":"Umbenannt","slug":"mein-projekt"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	// Stale → 412
	rec = do("PUT", "/projects/"+id, `{"name":"x","slug":"mein-projekt"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("stale rename: want 412, got %d", rec.Code)
	}

	// Archivieren via PUT archived=true
	rec = do("PUT", "/projects/"+id, `{"name":"Umbenannt","slug":"mein-projekt","archived":true}`, map[string]string{"If-Match": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	rec = do("GET", "/projects", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 0 {
		t.Errorf("archived project still listed: %v", page.Items)
	}
	rec = do("GET", "/projects?all=1", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 {
		t.Errorf("?all=1 should include archived: %v", page.Items)
	}
}
