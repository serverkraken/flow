package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/handlers"
)

// — fixtures + helpers --------------------------------------------------------

func mkActionsDeps(store *sqliteserver.Store, now time.Time) handlers.SessionActionsDeps {
	clock := &testutil.FixedClock{T: now}
	view := &usecase.ServerWorktimeView{
		Sessions:      sqliteserver.NewSessions(store),
		Active:        sqliteserver.NewActiveSessions(store),
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}
	return handlers.SessionActionsDeps{
		Sessions:    sqliteserver.NewSessions(store),
		Active:      sqliteserver.NewActiveSessions(store),
		Projects:    sqliteserver.NewProjects(store),
		View:        view,
		Clock:       clock,
		DeviceLabel: "test-device",
	}
}

// actionReq builds an *http.Request with the user injected into context,
// a chi route context carrying the supplied URL params, and (for body
// shapes that need it) a form-encoded body. Mirrors what the chi router
// + browser middleware would do at runtime.
func actionReq(t *testing.T, method, target, body string, u domain.User, params map[string]string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := httpserver.WithUser(r.Context(), u)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return r.WithContext(ctx)
}

// — tests --------------------------------------------------------------------

func TestSessionEdit_GET_RendersFormWithPrefilledInputs(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-edit-1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 90*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/"+s.ID+"/edit", "", u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="session-form"`,
		`name="start"`,
		`name="stop"`,
		`name="date"`,
		`name="tag"`,
		`name="note"`,
		`value="09:00"`,
		`value="10:30"`,
	})
}

func TestSessionEdit_GET_Cancel_ReturnsRow(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-edit-cancel")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/"+s.ID+"/edit?cancel=1", "", u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="session-row"`) {
		t.Errorf("cancel must return read-only row, got: %s", rr.Body.String())
	}
}

func TestSessionEdit_GET_NotFound_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-edit-nf")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(store, now)
	h := handlers.NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/missing/edit", "", u, map[string]string{"id": "00000000-0000-0000-0000-000000000000"})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestSessionEdit_GET_Unauthorized_401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(store, now)
	h := handlers.NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime/sessions/x/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

func TestSessionPut_HappyPath_ReturnsUpdatedRow(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-put-1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("tag", "review")
	form.Set("note", "code review")
	form.Set("version", strconv.FormatInt(s.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+s.ID, form.Encode(), u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="session-row"`,
		`09:00 — 10:30`,
		`review`,
	})
}

func TestSessionPut_VersionConflict_RendersOverlay(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-put-conf")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("tag", "review")
	form.Set("version", "999") // stale

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+s.ID, form.Encode(), u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="conflict-overlay"`) {
		t.Errorf("conflict body missing overlay: %s", rr.Body.String())
	}
}

func TestSessionPut_BadInput_BadRequest_RendersForm(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-put-bad")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "10:30")
	form.Set("stop", "09:00") // stop before start
	form.Set("version", strconv.FormatInt(s.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+s.ID, form.Encode(), u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d got %d", http.StatusBadRequest, rr.Code)
	}
	mustContain(t, rr.Body.String(), []string{
		`data-testid="session-form"`,
	})
}

func TestSessionPut_DateOnlyChange_ReAnchorsTimes(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-put-date-only")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionPut(d)

	// Submit ONLY a new date — leave start/stop unset. The handler must
	// re-anchor the preserved wall-clock times to the new date so the row
	// stays internally consistent (date column == start.Date()).
	form := url.Values{}
	form.Set("date", "2026-06-05")
	form.Set("version", strconv.FormatInt(s.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+s.ID, form.Encode(), u, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	saved, err := sessions.GetByID(u.ID, s.ID)
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	wantDate := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	if !saved.Date.Equal(wantDate) {
		t.Errorf("Date: got %s, want %s", saved.Date, wantDate)
	}
	if y, m, day := saved.Start.Date(); y != 2026 || m != time.June || day != 5 {
		t.Errorf("Start.Date(): got %04d-%02d-%02d, want 2026-06-05", y, m, day)
	}
	if y, m, day := saved.Stop.Date(); y != 2026 || m != time.June || day != 5 {
		t.Errorf("Stop.Date(): got %04d-%02d-%02d, want 2026-06-05", y, m, day)
	}
	// Wall-clock times preserved.
	if saved.Start.Hour() != 9 || saved.Start.Minute() != 0 {
		t.Errorf("Start time: got %02d:%02d, want 09:00", saved.Start.Hour(), saved.Start.Minute())
	}
	if saved.Stop.Hour() != 10 || saved.Stop.Minute() != 0 {
		t.Errorf("Stop time: got %02d:%02d, want 10:00", saved.Stop.Hour(), saved.Stop.Minute())
	}
}

func TestSessionPut_CrossTenant_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	uA := seedUser(t, store, "sa-put-uA")
	uB := seedUser(t, store, "sa-put-uB")
	pA := seedProject(t, store, uA.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, uA.ID, pA.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("version", strconv.FormatInt(s.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+s.ID, form.Encode(), uB, map[string]string{"id": s.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant put: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestSessionDelete_HappyPath_Empty200(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-del-1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+s.ID, "", u, map[string]string{"id": s.ID})
	r.Header.Set("If-Match", strconv.FormatInt(s.Version, 10))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "" {
		t.Errorf("delete body must be empty, got: %s", rr.Body.String())
	}
	if _, err := sqliteserver.NewSessions(store).GetByID(u.ID, s.ID); err == nil {
		t.Errorf("session must be deleted")
	}
}

func TestSessionDelete_Conflict_RendersOverlay(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-del-conf")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+s.ID, "", u, map[string]string{"id": s.ID})
	r.Header.Set("If-Match", "999")
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="conflict-overlay"`) {
		t.Errorf("conflict body missing overlay")
	}
}

func TestSessionDelete_CrossTenant_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	uA := seedUser(t, store, "sa-del-uA")
	uB := seedUser(t, store, "sa-del-uB")
	pA := seedProject(t, store, uA.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := sqliteserver.NewSessions(store)
	s := seedSession(t, sessions, uA.ID, pA.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(store, now)
	h := handlers.NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+s.ID, "", uB, map[string]string{"id": s.ID})
	r.Header.Set("If-Match", strconv.FormatInt(s.Version, 10))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant delete: got %d, want 404", rr.Code)
	}
}

func TestActiveStart_HappyPath_ReturnsBanner(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-start-1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStart(d)
	rr := httptest.NewRecorder()

	form := url.Values{}
	form.Set("project_id", p.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), u, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`id="live-banner-container"`,
		"webui-mockups",
		"aktive session",
	})
}

func TestActiveStart_AlreadyActive_409(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-start-conf")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	active := sqliteserver.NewActiveSessions(store)
	if _, err := active.Start(u.ID, p.ID, "mac", 0, "", ""); err != nil {
		t.Fatalf("seed Start: %v", err)
	}

	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStart(d)
	rr := httptest.NewRecorder()

	form := url.Values{}
	form.Set("project_id", p.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), u, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409", rr.Code)
	}
}

func TestActiveStart_CrossTenantProject_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	uA := seedUser(t, store, "sa-start-uA")
	uB := seedUser(t, store, "sa-start-uB")
	pA := seedProject(t, store, uA.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStart(d)
	rr := httptest.NewRecorder()
	form := url.Values{}
	form.Set("project_id", pA.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), uB, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant start: got %d, want 404", rr.Code)
	}
}

func TestActiveStop_HappyPath_Empty(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-stop-1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	active := sqliteserver.NewActiveSessions(store)
	if _, err := active.Start(u.ID, p.ID, "mac", 0, "", ""); err != nil {
		t.Fatalf("seed Start: %v", err)
	}

	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStop(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/stop", "", u, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="live-banner-container"`) {
		t.Errorf("stop must return empty container, got: %s", body)
	}
	if strings.Contains(body, "aktive session") {
		t.Errorf("stop body must not contain active banner, got: %s", body)
	}
}

func TestActiveStop_NoActive_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-stop-none")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStop(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/stop", "", u, nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

func TestActiveStart_MissingProjectID_400(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "sa-start-miss")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(store, now)
	h := handlers.NewActiveStart(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/start", "", u, nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rr.Code)
	}
}

// — helpers ------------------------------------------------------------------

func mustContain(t *testing.T, body string, parts []string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Errorf("body missing %q\nbody=%s", p, body)
		}
	}
}
