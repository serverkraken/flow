package httpserver

// Integration-style tests for the projects HTTP handlers. They wire a real
// *sqliteserver.Projects against a t.TempDir() SQLite database so that the
// optimistic-concurrency (Lamport) semantics are exercised end-to-end.
//
// A fake ProjectsServer is NOT used here because mocking out Upsert would hide
// the critical OCC behaviour (version bumping, conflict detection). Real
// sqliteserver gives confidence that the handler wiring is correct.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
)

// mustOpenServerProjects opens a fresh sqliteserver.Store in t.TempDir() and
// returns a ready *sqliteserver.Projects together with a pre-provisioned user.
func mustOpenServerProjects(t *testing.T) (*sqliteserver.Projects, domain.User) {
	t.Helper()
	dir := t.TempDir()
	store, err := sqliteserver.Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("sqliteserver.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	users := sqliteserver.NewUsers(store)
	u, err := users.EnsureBySub("sub|test", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	return sqliteserver.NewProjects(store), u
}

// pullRequest builds a GET /api/v1/projects request with the given user in
// context and since/limit query params.
func pullRequest(t *testing.T, u domain.User, since int64, limit int) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	q := req.URL.Query()
	if since != 0 {
		q.Set("since", jsonInt(since))
	}
	if limit > 0 {
		q.Set("limit", jsonInt(int64(limit)))
	}
	req.URL.RawQuery = q.Encode()
	return req.WithContext(WithUser(req.Context(), u))
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// pushRequest builds a PUT /api/v1/projects/{id} request with the given user
// in context, If-Match header, and a JSON body.
func pushRequest(t *testing.T, u domain.User, id string, expectedVersion int64, body domain.Project) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+id, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", jsonInt(expectedVersion))

	// chi.URLParam reads from the chi route context keyed by chi.RouteCtxKey.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := WithUser(req.Context(), u)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// decodePullResp decodes the pull response body.
func decodePullResp(t *testing.T, rr *httptest.ResponseRecorder) (items []domain.Project, highWatermark int64, hasMore bool) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items         []domain.Project `json:"items"`
		HighWatermark int64            `json:"high_watermark"`
		HasMore       bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode pull response: %v", err)
	}
	return payload.Items, payload.HighWatermark, payload.HasMore
}

// decodePushResp decodes a successful PUT response body.
func decodePushResp(t *testing.T, rr *httptest.ResponseRecorder) (id string, version int64) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode push response: %v", err)
	}
	return payload.ID, payload.Version
}

// -- Tests -------------------------------------------------------------------

func TestUnit_ProjectsPull_EmptyStore_Returns200WithEmptyPayload(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	handler := NewProjectsPullHandler(projects)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, pullRequest(t, u, 0, 0))

	items, hw, hasMore := decodePullResp(t, rr)
	if len(items) != 0 {
		t.Errorf("items len = %d, want 0", len(items))
	}
	if hw != 0 {
		t.Errorf("high_watermark = %d, want 0", hw)
	}
	if hasMore {
		t.Errorf("has_more = true, want false")
	}
}

func TestUnit_ProjectsPull_ThreeProjects_Since0_ReturnsAll(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	now := time.Now().UTC()

	var maxVersion int64
	for i := 0; i < 3; i++ {
		p, err := projects.Upsert(domain.Project{
			ID:        uuid.NewString(),
			UserID:    u.ID,
			Name:      "proj",
			Slug:      uuid.NewString(),
			CreatedAt: now,
		}, 0)
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		if p.Version > maxVersion {
			maxVersion = p.Version
		}
	}

	handler := NewProjectsPullHandler(projects)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, pullRequest(t, u, 0, 0))

	items, hw, hasMore := decodePullResp(t, rr)
	if len(items) != 3 {
		t.Errorf("items len = %d, want 3", len(items))
	}
	if hw != maxVersion {
		t.Errorf("high_watermark = %d, want %d", hw, maxVersion)
	}
	if hasMore {
		t.Errorf("has_more = true, want false")
	}
}

func TestUnit_ProjectsPull_SinceN_ReturnsOnlyNewer(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	now := time.Now().UTC()

	// Insert 3 projects; capture the version of the first.
	var firstVersion int64
	for i := 0; i < 3; i++ {
		p, err := projects.Upsert(domain.Project{
			ID:        uuid.NewString(),
			UserID:    u.ID,
			Name:      "proj",
			Slug:      uuid.NewString(),
			CreatedAt: now,
		}, 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		if i == 0 {
			firstVersion = p.Version
		}
	}

	handler := NewProjectsPullHandler(projects)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, pullRequest(t, u, firstVersion, 0))

	items, _, _ := decodePullResp(t, rr)
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2 (version > firstVersion)", len(items))
	}
}

func TestUnit_ProjectsPull_Limit2_Has3_HasMoreTrue(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	now := time.Now().UTC()

	var versions []int64
	for i := 0; i < 3; i++ {
		p, err := projects.Upsert(domain.Project{
			ID:        uuid.NewString(),
			UserID:    u.ID,
			Name:      "proj",
			Slug:      uuid.NewString(),
			CreatedAt: now,
		}, 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		versions = append(versions, p.Version)
	}

	handler := NewProjectsPullHandler(projects)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, pullRequest(t, u, 0, 2))

	items, hw, hasMore := decodePullResp(t, rr)
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2 (limit=2)", len(items))
	}
	if !hasMore {
		t.Errorf("has_more = false, want true")
	}
	// high_watermark must equal the version of the 2nd item (last returned).
	if hw != versions[1] {
		t.Errorf("high_watermark = %d, want version of 2nd item %d", hw, versions[1])
	}
}

func TestUnit_ProjectsPush_Insert_Returns200WithVersion(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	handler := NewProjectsPushHandler(projects)

	id := uuid.NewString()
	body := domain.Project{
		Name:      "New Project",
		Slug:      "new-project",
		CreatedAt: time.Now().UTC(),
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, pushRequest(t, u, id, 0, body))

	gotID, gotVersion := decodePushResp(t, rr)
	if gotID != id {
		t.Errorf("id = %q, want %q", gotID, id)
	}
	if gotVersion == 0 {
		t.Errorf("version = 0, want > 0")
	}
}

func TestUnit_ProjectsPush_UpdateCorrectVersion_Returns200(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	handler := NewProjectsPushHandler(projects)

	now := time.Now().UTC()
	id := uuid.NewString()

	// First: insert
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, pushRequest(t, u, id, 0, domain.Project{
		Name: "Initial", Slug: "initial", CreatedAt: now,
	}))
	_, v1 := decodePushResp(t, rr1)

	// Second: update with correct If-Match
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, pushRequest(t, u, id, v1, domain.Project{
		Name: "Updated", Slug: "initial", CreatedAt: now,
	}))
	_, v2 := decodePushResp(t, rr2)

	if v2 <= v1 {
		t.Errorf("version must bump: v1=%d v2=%d", v1, v2)
	}
}

func TestUnit_ProjectsPush_StaleIfMatch_Returns409WithCurrent(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	handler := NewProjectsPushHandler(projects)

	now := time.Now().UTC()
	id := uuid.NewString()

	// Insert first.
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, pushRequest(t, u, id, 0, domain.Project{
		Name: "Initial", Slug: "stale-test", CreatedAt: now,
	}))
	if rr1.Code != http.StatusOK {
		t.Fatalf("insert status = %d, want 200", rr1.Code)
	}

	// Attempt update with wrong version (0 again → stale).
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, pushRequest(t, u, id, 0, domain.Project{
		Name: "Conflict", Slug: "stale-test", CreatedAt: now,
	}))

	if rr2.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", rr2.Code)
	}
	var payload struct {
		Current *domain.Project `json:"current"`
	}
	if err := json.NewDecoder(rr2.Body).Decode(&payload); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if payload.Current == nil {
		t.Errorf("409 body missing 'current' field")
	} else if payload.Current.ID != id {
		t.Errorf("current.ID = %q, want %q", payload.Current.ID, id)
	}
}

func TestUnit_ProjectsPush_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	projects, u := mustOpenServerProjects(t)
	handler := NewProjectsPushHandler(projects)

	id := uuid.NewString()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+id, bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", "0")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(WithUser(req.Context(), u), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUnit_ProjectsNoBearer_Returns401(t *testing.T) {
	t.Parallel()
	projects, _ := mustOpenServerProjects(t)

	// Wire a chi router exactly as server.go's NewWithAuth does.
	r := chi.NewRouter()
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true}, nil))
		rr.Get("/api/v1/projects", NewProjectsPullHandler(projects).ServeHTTP)
		rr.Put("/api/v1/projects/{id}", NewProjectsPushHandler(projects).ServeHTTP)
	})

	// GET without Authorization header.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("GET without bearer: status = %d, want 401", rr.Code)
	}

	// PUT without Authorization header.
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+uuid.NewString(), nil))
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("PUT without bearer: status = %d, want 401", rr2.Code)
	}
}
