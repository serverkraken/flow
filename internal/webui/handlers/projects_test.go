package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/handlers"
)

// mkProjectsDeps assembles the handler deps off a server store. Tests
// reuse mustOpenServerStore + seedUser from dashboard_test.go.
func mkProjectsDeps(store *sqliteserver.Store, now time.Time) handlers.ProjectsDeps {
	clock := &testutil.FixedClock{T: now}
	return handlers.ProjectsDeps{
		Projects: sqliteserver.NewProjects(store),
		Sessions: sqliteserver.NewSessions(store),
		Active:   sqliteserver.NewActiveSessions(store),
		Clock:    clock,
	}
}

func projectsReqWithUser(t *testing.T, target string, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

func archiveProject(t *testing.T, store *sqliteserver.Store, userID, projectID string) {
	t.Helper()
	if err := sqliteserver.NewProjects(store).Archive(userID, projectID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
}

// — index tests —

func TestProjectsIndex_ListsActiveAndArchived(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "proj-list")
	// Thursday at 14:00.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	active := seedProject(t, store, u.ID, "webui-mockups")
	archived := seedProject(t, store, u.ID, "inbox-zero")
	archiveProject(t, store, u.ID, archived.ID)

	// Two completed sessions in webui-mockups this week, one in
	// inbox-zero (before it was archived).
	sessions := sqliteserver.NewSessions(store)
	seedSession(t, sessions, u.ID, active.ID, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), 90*time.Minute)
	seedSession(t, sessions, u.ID, active.ID, time.Date(2026, 6, 2, 14, 0, 0, 0, time.UTC), 60*time.Minute)
	seedSession(t, sessions, u.ID, archived.ID, time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), 30*time.Minute)

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		"webui-mockups",                       // active row name
		"inbox-zero",                          // archived row name
		`data-testid="projects-list"`,         // populated branch
		`data-testid="projects-totals"`,       // eyebrow
		"2 Projekte",                          // total label
		"2 aktiv letzte 7 Tage",               // eyebrow active count
		"aktiv letzte 7 Tage",                 // eyebrow tail
		"Zuletzt", "Diese Woche", "Sessions", // grid header
		"is-archived",                         // archived styling marker
		"2:30",                                // 1h30 + 1h = 2:30 active week sum
		`+ Neues Projekt`,                     // M7 disabled button
		`aria-disabled="true"`,                // disabled M7 button
		"Worktime-Projekte",                   // sub-tab label
		"Quellverzeichnisse",                  // sub-tab label
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("index body missing %q", s)
		}
	}
}

func TestProjectsIndex_Empty_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "proj-empty")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="projects-empty"`) {
		t.Errorf("empty-state placeholder missing; body=%s", body)
	}
	if !strings.Contains(body, "Noch keine Projekte synchronisiert") {
		t.Errorf("empty-state copy missing")
	}
	if !strings.Contains(body, "0 Projekte") {
		t.Errorf("zero-count label missing")
	}
}

func TestProjectsIndex_ActiveSession_RendersRunningGlyph(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "proj-active")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	p := seedProject(t, store, u.ID, "live-project")
	active := sqliteserver.NewActiveSessions(store)
	if _, err := active.Start(u.ID, p.ID, "laptop", 0, "design", ""); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "▶") {
		t.Errorf("running glyph missing")
	}
	if !strings.Contains(body, "jetzt") {
		t.Errorf("'jetzt' label missing for active project row")
	}
	if !strings.Contains(body, "is-running") {
		t.Errorf("is-running modifier missing for active row")
	}
}

func TestProjectsIndex_UserIsolation(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	owner := seedUser(t, store, "proj-iso-owner")
	other := seedUser(t, store, "proj-iso-other")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	_ = seedProject(t, store, owner.ID, "owned-project")
	_ = seedProject(t, store, other.ID, "leaked-project")

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", owner))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "owned-project") {
		t.Errorf("owner cannot see own project")
	}
	if strings.Contains(body, "leaked-project") {
		t.Errorf("cross-tenant leak: 'leaked-project' appears in owner's body")
	}
}

func TestProjectsIndex_QuellenTab_ShowsPlaceholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "proj-quellen")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	_ = seedProject(t, store, u.ID, "irrelevant")

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects?tab=quellen", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="projects-quellen-placeholder"`) {
		t.Errorf("quellen placeholder missing; body=%s", body)
	}
	if !strings.Contains(body, "über die TUI gepflegt") {
		t.Errorf("quellen placeholder copy missing")
	}
	// And: project rows should NOT render in the quellen tab.
	if strings.Contains(body, `data-testid="projects-row"`) {
		t.Errorf("project rows leaked into quellen tab")
	}
}

func TestProjects_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewProjects(mkProjectsDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/projects", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user on /projects: got %d, want 401", rr.Code)
	}
}
