// internal/adapter/httpserver/worktime_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

type worktimeAPIEnv struct {
	user   domain.User
	projID string
	router chi.Router
}

// newWorktimeAPIEnv wires the new API handlers onto a bare chi router with
// the test user pre-injected — mirrors what Task 18 mounts in production.
func newWorktimeAPIEnv(t *testing.T, sub string) worktimeAPIEnv {
	t.Helper()
	users := pgstore.NewUsers(pgTestStore)
	u, err := users.EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	proj, err := pgstore.NewProjects(pgTestStore).EnsureBySlug(u.ID, "Work", "work")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	deps := WorktimeAPIDeps{
		Sessions: pgstore.NewSessions(pgTestStore),
		Active:   pgstore.NewActiveSessions(pgTestStore, pgstore.NewSessions(pgTestStore), pgstore.NewSettings(pgTestStore)),
		Settings: pgstore.NewSettings(pgTestStore),
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountWorktimeAPI(r, deps)
	return worktimeAPIEnv{user: u, projID: proj.ID, router: r}
}

func (e worktimeAPIEnv) do(t *testing.T, method, path string, body any, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func TestWorktimeAPI_ActiveLifecycle(t *testing.T) {
	e := newWorktimeAPIEnv(t, "api-active-1")

	// Start
	rec := e.do(t, "POST", "/worktime/active/start",
		map[string]string{"project_id": e.projID, "tag": "deep"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	// Doppel-Start → 409 (Spec §7)
	rec = e.do(t, "POST", "/worktime/active/start",
		map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("double start: want 409, got %d", rec.Code)
	}
	// GET active
	rec = e.do(t, "GET", "/worktime/active", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("active list: %d", rec.Code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("active items: %d", len(list.Items))
	}
	// Pause → paused_at gesetzt; idempotent
	rec = e.do(t, "POST", "/worktime/active/pause", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(t, "POST", "/worktime/active/pause", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause idempotent: %d", rec.Code)
	}
	// Resume
	rec = e.do(t, "POST", "/worktime/active/resume", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume: %d", rec.Code)
	}
	// Stop → Session-DTO
	rec = e.do(t, "POST", "/worktime/active/stop", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body)
	}
	var sess map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess["id"] == "" || sess["project_id"] != e.projID {
		t.Errorf("stop payload: %v", sess)
	}
	// Stop ohne aktive → 404
	rec = e.do(t, "POST", "/worktime/active/stop", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("double stop: want 404, got %d", rec.Code)
	}
}

func TestWorktimeAPI_SessionsCRUDAndBulk(t *testing.T) {
	e := newWorktimeAPIEnv(t, "api-sess-1")

	// Manuelle Session (Nachtrag)
	create := map[string]any{
		"project_id": e.projID,
		"started_at": "2026-06-10T09:00:00Z",
		"stopped_at": "2026-06-10T10:30:00Z",
		"tag":        "deep",
	}
	rec := e.do(t, "POST", "/worktime/sessions", create, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" || created["day"] != "2026-06-10" {
		t.Fatalf("create payload: %v", created)
	}

	// Liste im Zeitraum
	rec = e.do(t, "GET", "/worktime/sessions?from=2026-06-10&to=2026-06-10", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	// Validierung: kaputtes from → 422
	rec = e.do(t, "GET", "/worktime/sessions?from=gestern&to=2026-06-10", nil, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad from: want 422, got %d", rec.Code)
	}

	// PUT mit If-Match
	update := map[string]any{
		"project_id": e.projID,
		"started_at": "2026-06-10T09:00:00Z",
		"stopped_at": "2026-06-10T11:00:00Z",
		"note":       "korrigiert",
	}
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// Stale If-Match → 412 + current
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale put: want 412, got %d", rec.Code)
	}
	// Fehlender If-Match → 422
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing if-match: want 422, got %d", rec.Code)
	}

	// DELETE mit If-Match
	rec = e.do(t, "DELETE", "/worktime/sessions/"+id, nil, map[string]string{"If-Match": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}

	// Bulk idempotent (Client-UUIDv5)
	bulk := map[string]any{"sessions": []map[string]any{
		{
			"id": "8c5e9b7e-0000-5000-8000-000000000001", "project_id": e.projID,
			"started_at": "2026-01-05T09:00:00Z", "stopped_at": "2026-01-05T10:00:00Z",
		},
	}}
	for i := 0; i < 2; i++ {
		rec = e.do(t, "POST", "/worktime/sessions:bulk", bulk, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("bulk run %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	rec = e.do(t, "GET", "/worktime/sessions?from=2026-01-01&to=2026-01-31", nil, nil)
	var page struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 {
		t.Errorf("bulk idempotency: want 1 session, got %d", len(page.Items))
	}
	fmt.Sprint() // keep fmt import
}
