package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

// mkProjectsDeps assembles the handler deps off pgStores.
func mkProjectsDeps(s pgStores, now time.Time) ProjectsDeps {
	clock := &testutil.FixedClock{T: now}
	return ProjectsDeps{
		Projects: s.Projects,
		Sessions: s.Sessions,
		Active:   s.Active,
		Clock:    clock,
	}
}

func projectsReqWithUser(t *testing.T, target string, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

func archiveProject(t *testing.T, s pgStores, projectID string) {
	t.Helper()
	if err := s.Projects.Archive(s.User.ID, projectID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
}

// — index tests —

func TestProjectsIndex_ListsActiveAndArchived(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "proj-list")
	// Thursday at 14:00.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	active := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	archived := seedProject(t, s.Projects, s.User.ID, "inbox-zero")
	archiveProject(t, s, archived.ID)

	// Two completed sessions in webui-mockups this week, one in
	// inbox-zero (before it was archived).
	seedSession(t, s.Sessions, s.User.ID, active.ID, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), 90*time.Minute)
	seedSession(t, s.Sessions, s.User.ID, active.ID, time.Date(2026, 6, 2, 14, 0, 0, 0, time.UTC), 60*time.Minute)
	seedSession(t, s.Sessions, s.User.ID, archived.ID, time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), 30*time.Minute)

	h := NewProjects(mkProjectsDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"webui-mockups",                      // active row name
		"inbox-zero",                         // archived row name
		`data-testid="projects-list"`,        // populated branch
		`data-testid="projects-totals"`,      // eyebrow
		"2 Projekte",                         // total label
		"2 aktiv letzte 7 Tage",              // eyebrow active count
		"aktiv letzte 7 Tage",                // eyebrow tail
		"Zuletzt", "Diese Woche", "Sessions", // grid header
		"is-archived",          // archived styling marker
		"2:30",                 // 1h30 + 1h = 2:30 active week sum
		`+ Neues Projekt`,      // M7 disabled button
		`aria-disabled="true"`, // disabled M7 button
		"Worktime-Projekte",    // sub-tab label
		"Quellverzeichnisse",   // sub-tab label
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index body missing %q", want)
		}
	}
}

func TestProjectsIndex_Empty_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "proj-empty")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewProjects(mkProjectsDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", s.User))

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
	s := newPGStores(t, "proj-active")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	p := seedProject(t, s.Projects, s.User.ID, "live-project")
	if _, err := s.Active.Start(s.User.ID, p.ID, time.Time{}, "laptop", 0, "design", ""); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	h := NewProjects(mkProjectsDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", s.User))

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
	owner := newPGStores(t, "proj-iso-owner")
	other := newPGStores(t, "proj-iso-other")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	seedProject(t, owner.Projects, owner.User.ID, "owned-project")
	seedProject(t, other.Projects, other.User.ID, "leaked-project")

	h := NewProjects(mkProjectsDeps(owner, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects", owner.User))

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
	s := newPGStores(t, "proj-quellen")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedProject(t, s.Projects, s.User.ID, "irrelevant")

	h := NewProjects(mkProjectsDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, projectsReqWithUser(t, "/projects?tab=quellen", s.User))

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
	s := newPGStores(t, "proj-nouser")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewProjects(mkProjectsDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/projects", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user on /projects: got %d, want 401", rr.Code)
	}
}
