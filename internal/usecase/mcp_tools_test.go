package usecase_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeWorktimeReader is a tiny stub for the WorktimeStatusReader
// interface. The real *usecase.WorktimeReader pulls in
// TargetResolver + ConfigReader + DayOffStore + LegacyActiveStore;
// tests don't need that machinery to verify the MCP status text.
type fakeWorktimeReader struct {
	day domain.Day
	err error
}

func (f *fakeWorktimeReader) Today() (domain.Day, error) { return f.day, f.err }

// mkTools wires a fully-functional MCPTools backed by the in-memory
// fakes already defined in other _test files in this package.
// authed controls whether the auth gate is open.
func mkTools(t *testing.T, authed bool) (*usecase.MCPTools, *fakeRepoStore, *fakeRepoNoteStore, *fakeActiveSessionStore, *fakeASProjectStore) {
	t.Helper()
	repos := newFakeRepoStore()
	notes := newFakeRepoNoteStore()
	queue := &fakeWriteQueue{}
	repoNotes := usecase.NewRepoNotes(repos, notes, queue, fakeResolverPkg{url: "git@github.com:acme/widget.git", ok: true})

	active := newFakeActiveSessionStore()
	projects := &fakeASProjectStore{}
	sessions := &fakeASSessionStore{}
	activeUC := usecase.NewActiveSessions(nil, projects, active, sessions, queue)
	sessionsUC := usecase.NewSessions(nil, projects, sessions, nil)

	reader := &fakeWorktimeReader{day: domain.Day{
		Target: 8 * time.Hour, Logged: 90 * time.Minute,
	}}

	return &usecase.MCPTools{
		UserID:        "u1",
		Pwd:           "/home/me/code/widget",
		Authed:        authed,
		Notes:         repoNotes,
		Active:        activeUC,
		Sessions:      sessionsUC,
		Reader:        reader,
		RepoNoteStore: notes,
		ProjectStore:  projects,
	}, repos, notes, active, projects
}

// ---- Catalog ----

func TestMCPTools_Catalog_ShipsSevenTools(t *testing.T) {
	t.Parallel()
	m := &usecase.MCPTools{}
	cat := m.Catalog()
	if len(cat) != 7 {
		t.Fatalf("Catalog: got %d tools, want 7", len(cat))
	}
	wantNames := map[string]bool{
		"flow_get_repo_note":   true,
		"flow_save_repo_note":  true,
		"flow_list_repo_notes": true,
		"flow_search_notes":    true,
		"flow_worktime_status": true,
		"flow_start_session":   true,
		"flow_stop_session":    true,
	}
	for _, td := range cat {
		if !wantNames[td.Name] {
			t.Errorf("unexpected tool: %s", td.Name)
		}
		delete(wantNames, td.Name)
		if td.Description == "" {
			t.Errorf("%s: missing description", td.Name)
		}
		if td.InputSchema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want object", td.Name, td.InputSchema["type"])
		}
	}
	for n := range wantNames {
		t.Errorf("missing tool: %s", n)
	}
}

// ---- Auth gate ----

func TestMCPTools_Authed_False_ReturnsLoginRequired(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, false)
	for _, name := range []string{
		"flow_get_repo_note", "flow_save_repo_note", "flow_list_repo_notes",
		"flow_search_notes", "flow_worktime_status",
		"flow_start_session", "flow_stop_session",
	} {
		got := m.Call(name, map[string]any{"content": "x", "query": "x"})
		if !got.IsError {
			t.Errorf("%s: IsError=false, want true", name)
		}
		if !strings.Contains(got.Text, "Login required") {
			t.Errorf("%s: text = %q, want contains 'Login required'", name, got.Text)
		}
	}
}

func TestMCPTools_Authed_False_ResourceCatalogEmpty(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, false)
	if got := m.ResourceCatalog(); got != nil {
		t.Errorf("ResourceCatalog: got %d entries, want nil", len(got))
	}
}

// ---- Unknown tool ----

func TestMCPTools_UnknownTool(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_nonexistent", nil)
	if !got.IsError || !strings.Contains(got.Text, "unknown tool") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_get_repo_note ----

func TestMCPTools_GetRepoNote_NewRepo_ReturnsHint(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_get_repo_note", map[string]any{})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no RepoNote yet") {
		t.Errorf("text = %q, want 'no RepoNote yet'", got.Text)
	}
}

func TestMCPTools_GetRepoNote_ExistingNote_ReturnsBody(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	if _, err := m.Notes.Save("u1", "/home/me/code/widget", "# widget rules\nbe nice"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := m.Call("flow_get_repo_note", map[string]any{})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "be nice") {
		t.Errorf("text missing body: %q", got.Text)
	}
}

func TestMCPTools_GetRepoNote_NoPwd(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	m.Pwd = ""
	got := m.Call("flow_get_repo_note", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "PWD is empty") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_save_repo_note ----

func TestMCPTools_SaveRepoNote_RoundTrip(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_save_repo_note", map[string]any{"content": "hello"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "saved") {
		t.Errorf("text = %q", got.Text)
	}
	// Verify persistence by reading back.
	read := m.Call("flow_get_repo_note", map[string]any{})
	if !strings.Contains(read.Text, "hello") {
		t.Errorf("get after save missing content: %q", read.Text)
	}
}

func TestMCPTools_SaveRepoNote_MissingContent(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_save_repo_note", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "'content' is required") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_list_repo_notes ----

func TestMCPTools_ListRepoNotes_Empty(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_list_repo_notes", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "0 repo") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_ListRepoNotes_WithEntries(t *testing.T) {
	t.Parallel()
	m, repos, _, _, _ := mkTools(t, true)
	// Save a note → repo gets created + a note row exists.
	if _, err := m.Notes.Save("u1", "/home/me/code/widget", "alpha"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Bump the repo version so PullSince returns it (PullSince filters Version > since).
	for id, r := range repos.rows {
		r.Version = 1
		repos.rows[id] = r
	}
	got := m.Call("flow_list_repo_notes", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "widget") {
		t.Errorf("text missing repo: %q", got.Text)
	}
	if !strings.Contains(got.Text, "bytes=5") {
		t.Errorf("text missing byte count: %q", got.Text)
	}
}

// ---- flow_search_notes ----

func TestMCPTools_SearchNotes_MissingQuery(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_search_notes", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "'query' is required") {
		t.Fatalf("got %+v", got)
	}
}

func TestMCPTools_SearchNotes_NoMatches(t *testing.T) {
	t.Parallel()
	m, _, notes, _, _ := mkTools(t, true)
	notes.byID["n1"] = domain.RepoNote{ID: "n1", UserID: "u1", RepoID: "fake-repo-a", Content: "alpha beta", Version: 1}
	got := m.Call("flow_search_notes", map[string]any{"query": "zeta"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no matches") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_SearchNotes_FindsMatchCaseInsensitive(t *testing.T) {
	t.Parallel()
	m, repos, notes, _, _ := mkTools(t, true)
	// Seed a repo + note that PullSince(0, ...) will return.
	r, _ := repos.EnsureByCanonicalKey("u1", "git:github.com/acme/widget", "widget")
	repos.rows[r.ID] = domain.Repo{ID: r.ID, UserID: "u1", CanonicalKey: r.CanonicalKey, DisplayName: r.DisplayName, Version: 1}
	notes.byID["n1"] = domain.RepoNote{ID: "n1", UserID: "u1", RepoID: r.ID, Content: "ALPHA bravo charlie", Version: 1}
	notes.byRepo["u1|"+r.ID] = "n1"
	got := m.Call("flow_search_notes", map[string]any{"query": "alpha"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "1 match") {
		t.Errorf("text = %q", got.Text)
	}
}

// ---- flow_worktime_status ----

func TestMCPTools_WorktimeStatus_NoActive(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_worktime_status", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no active session") {
		t.Errorf("text = %q", got.Text)
	}
	if !strings.Contains(got.Text, "logged=1h30m0s") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_WorktimeStatus_WithActive(t *testing.T) {
	t.Parallel()
	m, _, _, active, projects := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	_ = active.Upsert(domain.ActiveSession{
		UserID: "u1", ProjectID: "p1",
		StartedAt:       time.Now().Add(-30 * time.Minute),
		StartedOnDevice: "macbook",
		Tag:             "deep",
	})
	got := m.Call("flow_worktime_status", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "Widget (widget)") {
		t.Errorf("text missing project name: %q", got.Text)
	}
	if !strings.Contains(got.Text, "macbook") {
		t.Errorf("text missing device: %q", got.Text)
	}
}

// ---- flow_start_session / flow_stop_session ----

func TestMCPTools_StartSession_HappyPath(t *testing.T) {
	t.Parallel()
	m, _, _, _, projects := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	got := m.Call("flow_start_session", map[string]any{"project": "widget", "tag": "deep"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "started project=Widget") || !strings.Contains(got.Text, "tag=\"deep\"") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_StartSession_AlreadyRunning(t *testing.T) {
	t.Parallel()
	m, _, _, active, projects := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	_ = active.Upsert(domain.ActiveSession{UserID: "u1", ProjectID: "p1", StartedAt: time.Now()})
	got := m.Call("flow_start_session", map[string]any{"project": "widget"})
	if !got.IsError || !strings.Contains(got.Text, "already running") {
		t.Fatalf("got %+v", got)
	}
}

func TestMCPTools_StopSession_HappyPath(t *testing.T) {
	t.Parallel()
	m, _, _, active, projects := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	_ = active.Upsert(domain.ActiveSession{UserID: "u1", ProjectID: "p1", StartedAt: time.Now().Add(-1 * time.Hour)})
	got := m.Call("flow_stop_session", map[string]any{"project": "widget"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "stopped project=Widget") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_StopSession_NotRunning(t *testing.T) {
	t.Parallel()
	m, _, _, _, projects := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	got := m.Call("flow_stop_session", map[string]any{"project": "widget"})
	if !got.IsError || !strings.Contains(got.Text, "no active session") {
		t.Fatalf("got %+v", got)
	}
}
