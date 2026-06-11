package handlers

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
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// — fixtures + helpers --------------------------------------------------------

func mkActionsDeps(s pgStores, now time.Time) SessionActionsDeps {
	clock := &testutil.FixedClock{T: now}
	view := &usecase.ServerWorktimeView{
		Sessions:      s.Sessions,
		Active:        s.Active,
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}
	return SessionActionsDeps{
		Sessions:    s.Sessions,
		Active:      s.Active,
		Projects:    s.Projects,
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
	s := newPGStores(t, "sa-edit-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 90*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/"+sess.ID+"/edit", "", s.User, map[string]string{"id": sess.ID})
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
	s := newPGStores(t, "sa-edit-cancel")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/"+sess.ID+"/edit?cancel=1", "", s.User, map[string]string{"id": sess.ID})
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
	s := newPGStores(t, "sa-edit-nf")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(s, now)
	h := NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodGet, "/worktime/sessions/missing/edit", "", s.User, map[string]string{"id": "00000000-0000-0000-0000-000000000000"})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestSessionEdit_GET_Unauthorized_401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-edit-unauth")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(s, now)
	h := NewSessionEdit(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime/sessions/x/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

func TestSessionPut_HappyPath_ReturnsUpdatedRow(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-put-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("tag", "review")
	form.Set("note", "code review")
	form.Set("version", strconv.FormatInt(sess.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+sess.ID, form.Encode(), s.User, map[string]string{"id": sess.ID})
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
	s := newPGStores(t, "sa-put-conf")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("tag", "review")
	form.Set("version", "999") // stale

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+sess.ID, form.Encode(), s.User, map[string]string{"id": sess.ID})
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
	s := newPGStores(t, "sa-put-bad")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "10:30")
	form.Set("stop", "09:00") // stop before start
	form.Set("version", strconv.FormatInt(sess.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+sess.ID, form.Encode(), s.User, map[string]string{"id": sess.ID})
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
	s := newPGStores(t, "sa-put-date-only")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-05")
	form.Set("version", strconv.FormatInt(sess.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+sess.ID, form.Encode(), s.User, map[string]string{"id": sess.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	saved, err := s.Sessions.GetByID(s.User.ID, sess.ID)
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
	sA := newPGStores(t, "sa-put-uA")
	sB := newPGStores(t, "sa-put-uB")
	pA := seedProject(t, sA.Projects, sA.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, sA.Sessions, sA.User.ID, pA.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(sA, now)
	h := NewSessionPut(d)

	form := url.Values{}
	form.Set("date", "2026-06-04")
	form.Set("start", "09:00")
	form.Set("stop", "10:30")
	form.Set("version", strconv.FormatInt(sess.Version, 10))

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPut, "/worktime/sessions/"+sess.ID, form.Encode(), sB.User, map[string]string{"id": sess.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant put: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestSessionDelete_HappyPath_Empty200(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-del-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+sess.ID, "", s.User, map[string]string{"id": sess.ID})
	r.Header.Set("If-Match", strconv.FormatInt(sess.Version, 10))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "" {
		t.Errorf("delete body must be empty, got: %s", rr.Body.String())
	}
	if _, err := s.Sessions.GetByID(s.User.ID, sess.ID); err == nil {
		t.Errorf("session must be deleted")
	}
}

func TestSessionDelete_Conflict_RendersOverlay(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-del-conf")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, s.Sessions, s.User.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(s, now)
	h := NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+sess.ID, "", s.User, map[string]string{"id": sess.ID})
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
	sA := newPGStores(t, "sa-del-uA")
	sB := newPGStores(t, "sa-del-uB")
	pA := seedProject(t, sA.Projects, sA.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sess := seedSession(t, sA.Sessions, sA.User.ID, pA.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 60*time.Minute)

	d := mkActionsDeps(sA, now)
	h := NewSessionDelete(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodDelete, "/worktime/sessions/"+sess.ID, "", sB.User, map[string]string{"id": sess.ID})
	r.Header.Set("If-Match", strconv.FormatInt(sess.Version, 10))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant delete: got %d, want 404", rr.Code)
	}
}

func TestActiveStart_HappyPath_ReturnsBanner(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-start-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	d := mkActionsDeps(s, now)
	h := NewActiveStart(d)
	rr := httptest.NewRecorder()

	form := url.Values{}
	form.Set("project_id", p.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), s.User, nil)
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
	s := newPGStores(t, "sa-start-conf")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	if _, err := s.Active.Start(s.User.ID, p.ID, time.Time{}, "mac", 0, "", ""); err != nil {
		t.Fatalf("seed Start: %v", err)
	}

	d := mkActionsDeps(s, now)
	h := NewActiveStart(d)
	rr := httptest.NewRecorder()

	form := url.Values{}
	form.Set("project_id", p.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409", rr.Code)
	}
}

func TestActiveStart_CrossTenantProject_404(t *testing.T) {
	t.Parallel()
	sA := newPGStores(t, "sa-start-uA")
	sB := newPGStores(t, "sa-start-uB")
	pA := seedProject(t, sA.Projects, sA.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	// sB's Projects store is used — it won't find pA
	d := mkActionsDeps(sB, now)
	h := NewActiveStart(d)
	rr := httptest.NewRecorder()
	form := url.Values{}
	form.Set("project_id", pA.ID)
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), sB.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant start: got %d, want 404", rr.Code)
	}
}

func TestActiveStop_HappyPath_Empty(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-stop-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	if _, err := s.Active.Start(s.User.ID, p.ID, time.Time{}, "mac", 0, "", ""); err != nil {
		t.Fatalf("seed Start: %v", err)
	}

	d := mkActionsDeps(s, now)
	h := NewActiveStop(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/stop", "", s.User, nil)
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
	s := newPGStores(t, "sa-stop-none")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(s, now)
	h := NewActiveStop(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/stop", "", s.User, nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

func TestActiveStart_MissingProjectID_400(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "sa-start-miss")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	d := mkActionsDeps(s, now)
	h := NewActiveStart(d)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/start", "", s.User, nil)
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
