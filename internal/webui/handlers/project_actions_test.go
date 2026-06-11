package handlers

// project_actions_test.go — Plan E · Task 13 (M7).
//
// Drives the six project-action handlers through the same actionReq
// pattern as session_actions_test.go: an *http.Request with the
// authenticated user injected into context plus the chi URL params the
// router would normally populate.
//
// One test at the bottom — TestRouter_POST_Projects_HitsCreateHandler —
// boots the FULL server via httpserver.NewWithAuth against a
// httptest.NewServer instance with a real cookie-encoded session, so a
// wiring regression (handler created but never mounted) fails loudly.

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// — fixtures + helpers --------------------------------------------------------

func mkProjectActionsDeps(s pgStores, now time.Time) ProjectActionsDeps {
	clock := &testutil.FixedClock{T: now}
	return ProjectActionsDeps{
		Projects: s.Projects,
		Clock:    clock,
	}
}

// paReq mirrors actionReq from session_actions_test.go: injects the
// caller via WithUser + carries chi URL params + optional form body.
func paReq(t *testing.T, method, target, body string, u domain.User, params map[string]string) *http.Request {
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

// — Create -------------------------------------------------------------------

func TestProjectCreate_HappyPath_RestoresButtonAndOOBRow(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-create-1")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectCreate(d)

	form := url.Values{}
	form.Set("name", "Flow Refactor")

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="new-project-btn"`, // button restored
		`hx-swap-oob="afterbegin"`,      // OOB row swap
		`data-testid="projects-row"`,    // new row
		`Flow Refactor`,                 // name rendered
		`flow-refactor`,                 // slug rendered
	})

	// Persisted with the slugified slug.
	got, err := s.Projects.GetBySlug(s.User.ID, "flow-refactor")
	if err != nil {
		t.Fatalf("GetBySlug after create: %v", err)
	}
	if got.Name != "Flow Refactor" {
		t.Errorf("name: got %q, want %q", got.Name, "Flow Refactor")
	}
}

func TestProjectCreate_EmptyName_400_RendersFormError(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-create-empty")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectCreate(d)

	form := url.Values{}
	form.Set("name", "   ") // whitespace-only → trimmed to empty

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rr.Code)
	}
	mustContain(t, rr.Body.String(), []string{
		`data-testid="new-project-form"`,
		`data-testid="new-project-form-error"`,
		`Projektnamen`,
	})
}

func TestProjectCreate_CollidingSlug_AppendsSuffix(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-create-coll")
	// Seed an existing project with slug "duplicate".
	if _, err := s.Projects.EnsureBySlug(s.User.ID, "Duplicate", "duplicate"); err != nil {
		t.Fatalf("seed EnsureBySlug: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectCreate(d)

	form := url.Values{}
	form.Set("name", "Duplicate") // slug "duplicate" already taken → must become "duplicate-2"

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "duplicate-2") {
		t.Errorf("expected slug duplicate-2 in body, got: %s", rr.Body.String())
	}
}

// — Edit (GET form) ----------------------------------------------------------

func TestProjectEdit_GET_RendersFormPrefilled(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-edit-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectEdit(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodGet, "/projects/"+p.ID+"/edit", "", s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	mustContain(t, rr.Body.String(), []string{
		`data-testid="project-form"`,
		`name="name"`,
		`value="webui-mockups"`,
		`hx-put="/projects/` + p.ID + `"`,
	})
}

func TestProjectEdit_GET_CancelReturnsRow(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-edit-cancel")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectEdit(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodGet, "/projects/"+p.ID+"/edit?cancel=1", "", s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="projects-row"`) {
		t.Errorf("cancel must return read-only row, got: %s", rr.Body.String())
	}
}

func TestProjectEdit_Archived_Returns400(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-edit-archived")
	p := seedProject(t, s.Projects, s.User.ID, "archived-project")
	if err := s.Projects.Archive(s.User.ID, p.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectEdit(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodGet, "/projects/"+p.ID+"/edit", "", s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectPut_Archived_Returns400(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-put-archived")
	p, err := s.Projects.EnsureBySlug(s.User.ID, "To Archive", "to-archive")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	if err := s.Projects.Archive(s.User.ID, p.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectPut(d)

	form := url.Values{}
	form.Set("name", "Renamed After Archive")
	form.Set("version", strconv.FormatInt(p.Version, 10))

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPut, "/projects/"+p.ID, form.Encode(), s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	// Name must NOT have been changed.
	saved, err := s.Projects.GetByID(s.User.ID, p.ID)
	if err != nil {
		t.Fatalf("post-read: %v", err)
	}
	if saved.Name != "To Archive" {
		t.Errorf("archived project name changed: got %q, want %q", saved.Name, "To Archive")
	}
}

func TestProjectEdit_GET_Unauthorized_401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-edit-unauth")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectEdit(d)

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/projects/anything/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

// — PUT (rename) -------------------------------------------------------------

func TestProjectPut_HappyPath_ReturnsUpdatedRow(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-put-1")
	p, err := s.Projects.EnsureBySlug(s.User.ID, "Old Name", "old-name")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectPut(d)

	form := url.Values{}
	form.Set("name", "New Name")
	form.Set("version", strconv.FormatInt(p.Version, 10))

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPut, "/projects/"+p.ID, form.Encode(), s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	mustContain(t, rr.Body.String(), []string{
		`data-testid="projects-row"`,
		`New Name`,
		`old-name`, // slug must remain stable
	})

	saved, err := s.Projects.GetByID(s.User.ID, p.ID)
	if err != nil {
		t.Fatalf("post-read: %v", err)
	}
	if saved.Name != "New Name" {
		t.Errorf("name: got %q, want New Name", saved.Name)
	}
	if saved.Slug != "old-name" {
		t.Errorf("slug must be stable on rename: got %q, want old-name", saved.Slug)
	}
}

func TestProjectPut_VersionConflict_409_RendersFormWithServerName(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-put-conf")
	p, err := s.Projects.EnsureBySlug(s.User.ID, "Server Side Name", "server-side-name")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectPut(d)

	form := url.Values{}
	form.Set("name", "Local Edit")
	form.Set("version", "999") // stale

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPut, "/projects/"+p.ID, form.Encode(), s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="project-form"`,
		`data-testid="project-form-error"`,
		`value="Server Side Name"`,
		`Versionskonflikt`,
	})
	if strings.Contains(body, `value="Local Edit"`) {
		t.Errorf("conflict body must show server value, not user's stale input")
	}
}

func TestProjectPut_EmptyName_400_RendersFormError(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-put-empty")
	p, err := s.Projects.EnsureBySlug(s.User.ID, "Original", "original")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectPut(d)

	form := url.Values{}
	form.Set("name", "")
	form.Set("version", strconv.FormatInt(p.Version, 10))

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPut, "/projects/"+p.ID, form.Encode(), s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rr.Code)
	}
	mustContain(t, rr.Body.String(), []string{
		`data-testid="project-form"`,
		`data-testid="project-form-error"`,
	})
}

func TestProjectPut_CrossTenant_404(t *testing.T) {
	t.Parallel()
	sA := newPGStores(t, "pa-put-uA")
	sB := newPGStores(t, "pa-put-uB")
	pA := seedProject(t, sA.Projects, sA.User.ID, "tenant-a-project")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(sA, now)
	h := NewProjectPut(d)

	form := url.Values{}
	form.Set("name", "Stolen")
	form.Set("version", strconv.FormatInt(pA.Version, 10))

	rr := httptest.NewRecorder()
	// uB tries to PUT uA's project.
	r := paReq(t, http.MethodPut, "/projects/"+pA.ID, form.Encode(), sB.User, map[string]string{"id": pA.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant put: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// — Archive ------------------------------------------------------------------

func TestProjectArchive_HappyPath_ReturnsArchivedRow(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-arch-1")
	p := seedProject(t, s.Projects, s.User.ID, "doomed-project")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectArchive(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects/"+p.ID+"/archive", "", s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="projects-row"`,
		`archiviert`,  // the row's "Zuletzt" cell renders the archive label
		`is-archived`, // glyph + name pick up the archived class
	})

	// Persistence: archived_at is now set.
	saved, err := s.Projects.GetByID(s.User.ID, p.ID)
	if err != nil {
		t.Fatalf("post-read: %v", err)
	}
	if saved.ArchivedAt == nil {
		t.Errorf("archived_at must be set, was nil")
	}
}

func TestProjectArchive_CrossTenant_404(t *testing.T) {
	t.Parallel()
	sA := newPGStores(t, "pa-arch-uA")
	sB := newPGStores(t, "pa-arch-uB")
	pA := seedProject(t, sA.Projects, sA.User.ID, "tenant-a-arch")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(sA, now)
	h := NewProjectArchive(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects/"+pA.ID+"/archive", "", sB.User, map[string]string{"id": pA.ID})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant archive: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}

	// Verify uA's project is untouched.
	saved, err := sA.Projects.GetByID(sA.User.ID, pA.ID)
	if err != nil {
		t.Fatalf("post-read: %v", err)
	}
	if saved.ArchivedAt != nil {
		t.Errorf("cross-tenant archive must not touch the row, got archived_at=%v", saved.ArchivedAt)
	}
}

// — New-form button↔form toggle ----------------------------------------------

func TestProjectNewForm_GET_RendersForm(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-newform-1")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectNewForm(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodGet, "/projects/new", "", s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="new-project-form"`) {
		t.Errorf("expected new-project-form, got: %s", rr.Body.String())
	}
}

func TestProjectNewCancel_GET_RestoresButton(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pa-newcancel-1")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := mkProjectActionsDeps(s, now)
	h := NewProjectNewCancel(d)

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodGet, "/projects/new/cancel", "", s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-testid="new-project-btn"`) {
		t.Errorf("expected new-project-btn, got: %s", rr.Body.String())
	}
}

// — Router-level wiring test -------------------------------------------------
//
// TestRouter_POST_Projects_HitsCreateHandler boots the full server via
// NewWithAuth + httptest.NewServer, encodes a session cookie via the
// real Session.Encode, and POSTs /projects. Catches the class of
// regressions where a handler exists but is never mounted in
// server.go's route table.

func TestRouter_POST_Projects_HitsCreateHandler(t *testing.T) {
	t.Parallel()

	s := newPGStores(t, "pa-router")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	clock := &testutil.FixedClock{T: now}
	projects := s.Projects
	users := pgstore.NewUsers(pgWebUIStore)

	d := ProjectActionsDeps{Projects: projects, Clock: clock}

	webUI := &httpserver.WebUIHandlers{
		ProjectCreate: NewProjectCreate(d),
	}

	hashKey, _ := hex.DecodeString(strings.Repeat("11", 32))
	blockKey, _ := hex.DecodeString(strings.Repeat("22", 16))
	sess := httpserver.NewSession(hashKey, blockKey)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:     fakeProvider{id: ports.Identity{Sub: "u-router-test-pg", Email: "router@example", Name: "router"}},
		Access:       fakeAccess{ok: true},
		Session:      sess,
		Users:        users,
		WebUI:        webUI,
		BaseURL:      "http://localhost:0",
		OIDCClientID: "test-client",
		OIDCSecret:   "test-secret",
		Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: false},
		Ready:        func() error { return nil },
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// encodeTestSession is defined in sessioncookie_test.go (same package).
	cookieVal := encodeTestSession(t, sess, "flow_session", "u-router-test-pg", "router@example", "router", time.Hour)

	jar, _ := cookiejar.New(nil)
	tsURL, _ := url.Parse(ts.URL)
	jar.SetCookies(tsURL, []*http.Cookie{{Name: "flow_session", Value: cookieVal}})

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("name", "Router Wiring Smoke PG")

	resp, err := client.PostForm(ts.URL+"/projects", form)
	if err != nil {
		t.Fatalf("POST /projects: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /projects status: got %d, want 200 — wiring regression?", resp.StatusCode)
	}

	// The row must exist in the DB scoped to the user the
	// middleware EnsureBySub'd from our session sub.
	u, err := users.GetBySub("u-router-test-pg")
	if err != nil {
		t.Fatalf("GetBySub after request: %v", err)
	}
	got, err := projects.GetBySlug(u.ID, "router-wiring-smoke-pg")
	if err != nil {
		t.Fatalf("GetBySlug after request: %v", err)
	}
	if got.Name != "Router Wiring Smoke PG" {
		t.Errorf("created row name: got %q, want %q", got.Name, "Router Wiring Smoke PG")
	}
}

// fakeProvider + fakeAccess satisfy the AuthDeps interfaces without
// touching a real OIDC issuer. The router-level test only exercises
// the cookie-auth path, so Provider.Verify is never called.
type fakeProvider struct {
	id ports.Identity
}

func (f fakeProvider) Verify(_ context.Context, _ string) (ports.Identity, error) {
	return f.id, nil
}

func (fakeProvider) Endpoint() (authURL, tokenURL string) {
	return "https://idp.example/auth", "https://idp.example/token"
}
func (fakeProvider) DeviceAuthorizationURL() string { return "https://idp.example/device" }

type fakeAccess struct{ ok bool }

func (f fakeAccess) Allow(_ ports.Identity) bool { return f.ok }
