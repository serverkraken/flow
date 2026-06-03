package httpserver

// Integration-style tests for the sessions HTTP handlers. They wire a real
// *sqliteserver.Sessions against a t.TempDir() SQLite database so that the
// optimistic-concurrency (Lamport) semantics are exercised end-to-end.
//
// A fake SessionsServer is NOT used here because mocking out Upsert would hide
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

// sessionsTestDeps bundles what the sessions handler tests need.
type sessionsTestDeps struct {
	store  *sqliteserver.Sessions
	user   domain.User
	projID string // pre-seeded project UUID (satisfies sessions FK)
}

// mustOpenServerSessions opens a fresh sqliteserver.Store in t.TempDir() and
// returns a sessionsTestDeps with a pre-provisioned user and project.
func mustOpenServerSessions(t *testing.T) sessionsTestDeps {
	t.Helper()
	dir := t.TempDir()
	store, err := sqliteserver.Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("sqliteserver.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	users := sqliteserver.NewUsers(store)
	u, err := users.EnsureBySub("sub|sessions-test", "sessions@example.com", "Sessions User")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	// Sessions have a FK constraint to projects; provision a real row.
	projects := sqliteserver.NewProjects(store)
	p, err := projects.EnsureBySlug(u.ID, "sessions-test-project", "sessions-test-project")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	return sessionsTestDeps{
		store:  sqliteserver.NewSessions(store),
		user:   u,
		projID: p.ID,
	}
}

// sessionPullRequest builds a GET /api/v1/sessions request with the given user in
// context and since/limit query params.
func sessionPullRequest(t *testing.T, u domain.User, since int64, limit int) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
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

// sessionPushRequest builds a PUT /api/v1/sessions/{id} request.
func sessionPushRequest(t *testing.T, u domain.User, id string, expectedVersion int64, body domain.Session) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sessions/"+id, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", jsonInt(expectedVersion))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := WithUser(req.Context(), u)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// decodeSessionPullResp decodes the sessions pull response body.
func decodeSessionPullResp(t *testing.T, rr *httptest.ResponseRecorder) (items []domain.Session, highWatermark int64, hasMore bool) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items         []domain.Session `json:"items"`
		HighWatermark int64            `json:"high_watermark"`
		HasMore       bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode pull response: %v", err)
	}
	return payload.Items, payload.HighWatermark, payload.HasMore
}

// newTestSession returns a minimal session for testing using the pre-seeded projectID.
func newTestSession(userID, projectID string) domain.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now,
		Stop:      now.Add(30 * time.Minute),
		Elapsed:   30 * time.Minute,
	}
}

// -- Tests -------------------------------------------------------------------

func TestUnit_SessionsPull_EmptyStore_Returns200WithEmptyPayload(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)
	handler := NewSessionsPullHandler(d.store)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, sessionPullRequest(t, d.user, 0, 0))

	items, hw, hasMore := decodeSessionPullResp(t, rr)
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

func TestUnit_SessionsPull_ThreeSessions_Since0_ReturnsAll(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)

	var maxVersion int64
	for i := 0; i < 3; i++ {
		s, err := d.store.Upsert(newTestSession(d.user.ID, d.projID), 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		if s.Version > maxVersion {
			maxVersion = s.Version
		}
	}

	handler := NewSessionsPullHandler(d.store)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, sessionPullRequest(t, d.user, 0, 0))

	items, hw, hasMore := decodeSessionPullResp(t, rr)
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

func TestUnit_SessionsPull_SinceN_ReturnsOnlyNewer(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)

	var firstVersion int64
	for i := 0; i < 3; i++ {
		s, err := d.store.Upsert(newTestSession(d.user.ID, d.projID), 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		if i == 0 {
			firstVersion = s.Version
		}
	}

	handler := NewSessionsPullHandler(d.store)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, sessionPullRequest(t, d.user, firstVersion, 0))

	items, _, _ := decodeSessionPullResp(t, rr)
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2 (version > firstVersion)", len(items))
	}
}

func TestUnit_SessionsPull_Limit2_Has3_HasMoreTrue(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)

	var versions []int64
	for i := 0; i < 3; i++ {
		s, err := d.store.Upsert(newTestSession(d.user.ID, d.projID), 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		versions = append(versions, s.Version)
	}

	handler := NewSessionsPullHandler(d.store)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, sessionPullRequest(t, d.user, 0, 2))

	items, hw, hasMore := decodeSessionPullResp(t, rr)
	if len(items) != 2 {
		t.Errorf("items len = %d, want 2 (limit=2)", len(items))
	}
	if !hasMore {
		t.Errorf("has_more = false, want true")
	}
	if hw != versions[1] {
		t.Errorf("high_watermark = %d, want version of 2nd item %d", hw, versions[1])
	}
}

func TestUnit_SessionsPush_Insert_Returns200WithSession(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)
	handler := NewSessionsPushHandler(d.store)

	id := uuid.NewString()
	body := newTestSession(d.user.ID, d.projID)
	body.ID = id

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, sessionPushRequest(t, d.user, id, 0, body))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var out domain.Session
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.ID != id {
		t.Errorf("id = %q, want %q", out.ID, id)
	}
	if out.Version == 0 {
		t.Errorf("version = 0, want > 0")
	}
}

func TestUnit_SessionsPush_UpdateCorrectVersion_Returns200(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)
	handler := NewSessionsPushHandler(d.store)

	id := uuid.NewString()
	body := newTestSession(d.user.ID, d.projID)
	body.ID = id

	// First: insert.
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, sessionPushRequest(t, d.user, id, 0, body))
	if rr1.Code != http.StatusOK {
		t.Fatalf("insert status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}
	var out1 domain.Session
	if err := json.NewDecoder(rr1.Body).Decode(&out1); err != nil {
		t.Fatalf("decode insert response: %v", err)
	}
	v1 := out1.Version

	// Second: update with correct If-Match.
	body.Tag = "updated"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, sessionPushRequest(t, d.user, id, v1, body))
	if rr2.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	var out2 domain.Session
	if err := json.NewDecoder(rr2.Body).Decode(&out2); err != nil {
		t.Fatalf("decode update response: %v", err)
	}

	if out2.Version <= v1 {
		t.Errorf("version must bump: v1=%d v2=%d", v1, out2.Version)
	}
}

func TestUnit_SessionsPush_StaleIfMatch_Returns409WithCurrent(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)
	handler := NewSessionsPushHandler(d.store)

	id := uuid.NewString()
	body := newTestSession(d.user.ID, d.projID)
	body.ID = id

	// Insert first.
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, sessionPushRequest(t, d.user, id, 0, body))
	if rr1.Code != http.StatusOK {
		t.Fatalf("insert status = %d, want 200", rr1.Code)
	}

	// Attempt update with stale If-Match (0 again).
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, sessionPushRequest(t, d.user, id, 0, body))

	if rr2.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", rr2.Code)
	}
	var payload struct {
		Current *domain.Session `json:"current"`
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

func TestUnit_SessionsPush_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)
	handler := NewSessionsPushHandler(d.store)

	id := uuid.NewString()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sessions/"+id, bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", "0")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(WithUser(req.Context(), d.user), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUnit_SessionsNoBearer_Returns401(t *testing.T) {
	t.Parallel()
	d := mustOpenServerSessions(t)

	r := chi.NewRouter()
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(stubProv{}, stubAccessAll{allow: true}, nil))
		rr.Get("/api/v1/sessions", NewSessionsPullHandler(d.store).ServeHTTP)
		rr.Put("/api/v1/sessions/{id}", NewSessionsPushHandler(d.store).ServeHTTP)
	})

	// GET without Authorization header.
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("GET without bearer: status = %d, want 401", rr.Code)
	}

	// PUT without Authorization header.
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, httptest.NewRequest(http.MethodPut, "/api/v1/sessions/"+uuid.NewString(), nil))
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("PUT without bearer: status = %d, want 401", rr2.Code)
	}
}
