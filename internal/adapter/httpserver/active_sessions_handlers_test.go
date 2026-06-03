package httpserver

// Integration-style tests for the active_sessions HTTP handlers. They wire a
// real *sqliteserver.ActiveSessions against a t.TempDir() SQLite database so
// that the optimistic-concurrency (Lamport) semantics are exercised end-to-end.

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

// activeTestDeps holds everything the active_sessions handler tests need.
type activeTestDeps struct {
	store  *sqliteserver.ActiveSessions
	user   domain.User
	projID string
}

// mustOpenActiveServer opens a fresh sqliteserver.Store in t.TempDir() and
// returns an activeTestDeps with a pre-provisioned user and project.
func mustOpenActiveServer(t *testing.T) activeTestDeps {
	t.Helper()
	dir := t.TempDir()
	store, err := sqliteserver.Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("sqliteserver.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	users := sqliteserver.NewUsers(store)
	u, err := users.EnsureBySub("sub|active-test", "active@example.com", "Active User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	// Projects must exist because active_sessions has a FK to projects.
	projects := sqliteserver.NewProjects(store)
	p, err := projects.EnsureBySlug(u.ID, "test-project", "test-project")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	return activeTestDeps{
		store:  sqliteserver.NewActiveSessions(store),
		user:   u,
		projID: p.ID,
	}
}

// mustOpenActiveServerWithProjects opens like mustOpenActiveServer but also returns
// a *sqliteserver.Projects so the caller can provision additional project rows.
func mustOpenActiveServerWithProjects(t *testing.T) (activeTestDeps, *sqliteserver.Projects) {
	t.Helper()
	dir := t.TempDir()
	st, err := sqliteserver.Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("sqliteserver.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	users := sqliteserver.NewUsers(st)
	u, err := users.EnsureBySub("sub|active-multi", "multi@example.com", "Multi User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	projs := sqliteserver.NewProjects(st)
	p, err := projs.EnsureBySlug(u.ID, "test-project", "test-project")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	deps := activeTestDeps{
		store:  sqliteserver.NewActiveSessions(st),
		user:   u,
		projID: p.ID,
	}
	return deps, projs
}

// activeStartRequest builds a POST /api/v1/active/{project_id}/start request.
func activeStartRequest(t *testing.T, u domain.User, projectID string, expectedVersion int64, device string) *http.Request {
	t.Helper()
	body := map[string]string{}
	if device != "" {
		body["started_on_device"] = device
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/active/"+projectID+"/start", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", jsonInt(expectedVersion))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("project_id", projectID)
	ctx := context.WithValue(WithUser(req.Context(), u), chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// activeStopRequest builds a DELETE /api/v1/active/{project_id} request.
func activeStopRequest(t *testing.T, u domain.User, projectID string, expectedVersion int64, tag, note string) *http.Request {
	t.Helper()
	body := map[string]string{"tag": tag, "note": note}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/active/"+projectID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", jsonInt(expectedVersion))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("project_id", projectID)
	ctx := context.WithValue(WithUser(req.Context(), u), chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// activeListRequest builds a GET /api/v1/active request, optionally with ?since=N.
func activeListRequest(t *testing.T, u domain.User, since int64, useSince bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/active", nil)
	if useSince {
		q := req.URL.Query()
		q.Set("since", jsonInt(since))
		req.URL.RawQuery = q.Encode()
	}
	return req.WithContext(WithUser(req.Context(), u))
}

// -- Tests -------------------------------------------------------------------

// Test 1: Start when free + If-Match: 0 → 200 with active session JSON.
func TestUnit_ActiveStart_WhenFree_Returns200(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)
	handler := NewActiveStartHandler(d.store)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, activeStartRequest(t, d.user, d.projID, 0, "laptop"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var out domain.ActiveSession
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.ProjectID != d.projID {
		t.Errorf("project_id = %q, want %q", out.ProjectID, d.projID)
	}
	if out.Version == 0 {
		t.Errorf("version = 0, want > 0")
	}
	if out.StartedOnDevice != "laptop" {
		t.Errorf("started_on_device = %q, want laptop", out.StartedOnDevice)
	}
}

// Test 2: Start when already running + If-Match: 0 → 409 with {current}.
func TestUnit_ActiveStart_WhenAlreadyRunning_Returns409(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)
	startH := NewActiveStartHandler(d.store)

	// First start — should succeed.
	rr1 := httptest.NewRecorder()
	startH.ServeHTTP(rr1, activeStartRequest(t, d.user, d.projID, 0, "laptop"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first start status = %d, want 200", rr1.Code)
	}

	// Second start with If-Match: 0 while already running.
	rr2 := httptest.NewRecorder()
	startH.ServeHTTP(rr2, activeStartRequest(t, d.user, d.projID, 0, "phone"))

	if rr2.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", rr2.Code)
	}
	var payload struct {
		Current *domain.ActiveSession `json:"current"`
	}
	if err := json.NewDecoder(rr2.Body).Decode(&payload); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if payload.Current == nil {
		t.Errorf("409 body missing 'current' field")
	} else if payload.Current.ProjectID != d.projID {
		t.Errorf("current.ProjectID = %q, want %q", payload.Current.ProjectID, d.projID)
	}
}

// Test 3: Start with force-takeover + If-Match: matching version → 200, version bumped.
func TestUnit_ActiveStart_ForceTakeover_Returns200WithBumpedVersion(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)
	startH := NewActiveStartHandler(d.store)

	// First start.
	rr1 := httptest.NewRecorder()
	startH.ServeHTTP(rr1, activeStartRequest(t, d.user, d.projID, 0, "laptop"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first start status = %d, want 200", rr1.Code)
	}
	var first domain.ActiveSession
	if err := json.NewDecoder(rr1.Body).Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	// Force-takeover with correct If-Match.
	rr2 := httptest.NewRecorder()
	startH.ServeHTTP(rr2, activeStartRequest(t, d.user, d.projID, first.Version, "phone"))
	if rr2.Code != http.StatusOK {
		t.Fatalf("takeover status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	var second domain.ActiveSession
	if err := json.NewDecoder(rr2.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if second.Version <= first.Version {
		t.Errorf("version must bump: v1=%d v2=%d", first.Version, second.Version)
	}
	if second.StartedOnDevice != "phone" {
		t.Errorf("started_on_device = %q, want phone", second.StartedOnDevice)
	}
}

// Test 4: Stop happy path → 200, response is the created Session row.
// Subsequent GET /api/v1/active should not include that project.
func TestUnit_ActiveStop_HappyPath_Returns200AndClearsActive(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)

	// Start first.
	startH := NewActiveStartHandler(d.store)
	rr1 := httptest.NewRecorder()
	startH.ServeHTTP(rr1, activeStartRequest(t, d.user, d.projID, 0, "laptop"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200", rr1.Code)
	}
	var active domain.ActiveSession
	if err := json.NewDecoder(rr1.Body).Decode(&active); err != nil {
		t.Fatalf("decode start response: %v", err)
	}

	// Small sleep so Stop time > Start time at RFC3339 second resolution.
	time.Sleep(time.Millisecond)

	// Stop.
	stopH := NewActiveStopHandler(d.store)
	rr2 := httptest.NewRecorder()
	stopH.ServeHTTP(rr2, activeStopRequest(t, d.user, d.projID, active.Version, "deep", "done"))
	if rr2.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	var sess domain.Session
	if err := json.NewDecoder(rr2.Body).Decode(&sess); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if sess.ProjectID != d.projID {
		t.Errorf("sess.ProjectID = %q, want %q", sess.ProjectID, d.projID)
	}
	if sess.Tag != "deep" {
		t.Errorf("sess.Tag = %q, want deep", sess.Tag)
	}
	if sess.Version == 0 {
		t.Errorf("sess.Version = 0, want > 0")
	}

	// GET /api/v1/active should return empty after stop.
	listH := NewActiveListHandler(d.store)
	rr3 := httptest.NewRecorder()
	listH.ServeHTTP(rr3, activeListRequest(t, d.user, 0, false))
	if rr3.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rr3.Code)
	}
	var listPayload struct {
		Items []domain.ActiveSession `json:"items"`
	}
	if err := json.NewDecoder(rr3.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listPayload.Items) != 0 {
		t.Errorf("items len = %d, want 0 after stop", len(listPayload.Items))
	}
}

// Test 5: Stop with no active row → 404.
func TestUnit_ActiveStop_NoActiveRow_Returns404(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)
	handler := NewActiveStopHandler(d.store)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, activeStopRequest(t, d.user, d.projID, 0, "", ""))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// Test 6: Stop with stale If-Match → 409 with {current}.
func TestUnit_ActiveStop_StaleIfMatch_Returns409(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)

	// Start first.
	if _, err := d.store.Start(d.user.ID, d.projID, "laptop", 0, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stopH := NewActiveStopHandler(d.store)
	rr := httptest.NewRecorder()
	stopH.ServeHTTP(rr, activeStopRequest(t, d.user, d.projID, 999, "", ""))

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	var payload struct {
		Current *domain.ActiveSession `json:"current"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if payload.Current == nil {
		t.Errorf("409 body missing 'current' field")
	}
}

// Test 7: GET /api/v1/active (no since) → all currently-active items.
func TestUnit_ActiveList_NoSince_ReturnsAllActive(t *testing.T) {
	t.Parallel()
	d, projs := mustOpenActiveServerWithProjects(t)

	// Create a second project and start both.
	p2, err := projs.EnsureBySlug(d.user.ID, "project-2", "project-2")
	if err != nil {
		t.Fatalf("EnsureBySlug p2: %v", err)
	}

	for _, pID := range []string{d.projID, p2.ID} {
		if _, err := d.store.Start(d.user.ID, pID, "laptop", 0, "", ""); err != nil {
			t.Fatalf("Start %q: %v", pID, err)
		}
	}

	handler := NewActiveListHandler(d.store)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, activeListRequest(t, d.user, 0, false))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items []domain.ActiveSession `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Errorf("items len = %d, want 2", len(payload.Items))
	}
}

// Test 8: GET /api/v1/active?since=N → only items with version > N + high_watermark.
func TestUnit_ActiveList_WithSince_ReturnsOnlyNewer(t *testing.T) {
	t.Parallel()
	d, projs := mustOpenActiveServerWithProjects(t)

	p2, err := projs.EnsureBySlug(d.user.ID, "project-since-2", "project-since-2")
	if err != nil {
		t.Fatalf("EnsureBySlug p2: %v", err)
	}
	p3, err := projs.EnsureBySlug(d.user.ID, "project-since-3", "project-since-3")
	if err != nil {
		t.Fatalf("EnsureBySlug p3: %v", err)
	}

	var firstVersion int64
	for i, pID := range []string{d.projID, p2.ID, p3.ID} {
		a, err := d.store.Start(d.user.ID, pID, "laptop", 0, "", "")
		if err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
		if i == 0 {
			firstVersion = a.Version
		}
	}

	handler := NewActiveListHandler(d.store)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, activeListRequest(t, d.user, firstVersion, true))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items         []domain.ActiveSession `json:"items"`
		HighWatermark int64                  `json:"high_watermark"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Errorf("items len = %d, want 2 (version > firstVersion)", len(payload.Items))
	}
	if payload.HighWatermark == 0 {
		t.Errorf("high_watermark = 0, want > 0")
	}
}

// Test 9: No bearer → 401.
func TestUnit_ActiveNoBearer_Returns401(t *testing.T) {
	t.Parallel()
	d := mustOpenActiveServer(t)

	r := chi.NewRouter()
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true}, nil))
		rr.Get("/api/v1/active", NewActiveListHandler(d.store).ServeHTTP)
		rr.Post("/api/v1/active/{project_id}/start", NewActiveStartHandler(d.store).ServeHTTP)
		rr.Delete("/api/v1/active/{project_id}", NewActiveStopHandler(d.store).ServeHTTP)
	})

	// GET without Authorization header.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/active", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("GET without bearer: status = %d, want 401", rr.Code)
	}

	// POST without Authorization header.
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/api/v1/active/"+uuid.NewString()+"/start", nil))
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("POST without bearer: status = %d, want 401", rr2.Code)
	}

	// DELETE without Authorization header.
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, httptest.NewRequest(http.MethodDelete, "/api/v1/active/"+uuid.NewString(), nil))
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("DELETE without bearer: status = %d, want 401", rr3.Code)
	}
}
