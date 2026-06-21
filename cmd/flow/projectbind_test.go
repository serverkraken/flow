package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// --- helpers shared across bind tests ---

// newBindSrv creates an httptest server for binding tests.
// resolveProject: if non-nil, /projects/resolve returns it as JSON (200); otherwise 404.
func newBindSrv(t *testing.T, projects []domain.Project, bindings []domain.ProjectBinding) (srv *httptest.Server, putSlug *string, deletedPath *string) {
	srv, putSlug, deletedPath, _ = newBindSrvWithResolve(t, projects, bindings, nil)
	return srv, putSlug, deletedPath
}

func newBindSrvWithResolve(t *testing.T, projects []domain.Project, bindings []domain.ProjectBinding, resolveProject *domain.Project) (srv *httptest.Server, putSlug *string, deletedPath *string, resolveHit *bool) {
	t.Helper()
	var ps = projects
	var bs = bindings
	var putRemoteSlug string
	var dpath string
	var rHit bool
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(ps)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bindings"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			putRemoteSlug, _ = body["remoteSlug"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(domain.ProjectBinding{RemoteSlug: putRemoteSlug})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/bindings":
			dpath = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/bindings":
			_ = json.NewEncoder(w).Encode(bs)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/resolve":
			rHit = true
			if resolveProject == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(resolveProject)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &putRemoteSlug, &dpath, &rHit
}

// --- TestRunBind ---

func TestRunBind_Success(t *testing.T) {
	projects := []domain.Project{{ID: "p1", Name: "Acme", Slug: "acme"}}
	srv, putSlug, _ := newBindSrv(t, projects, nil)
	c := apiclient.New(srv.URL, "tkn")

	out, err := runBind(context.Background(), c, "github.com/acme/app", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if *putSlug != "github.com/acme/app" {
		t.Errorf("PUT remoteSlug = %q, want %q", *putSlug, "github.com/acme/app")
	}
	if !strings.Contains(out, "github.com/acme/app") || !strings.Contains(out, "acme") {
		t.Errorf("output missing expected strings: %q", out)
	}
}

func TestRunBind_UnknownSlug(t *testing.T) {
	projects := []domain.Project{{ID: "p1", Name: "Acme", Slug: "acme"}}
	srv, _, _ := newBindSrv(t, projects, nil)
	c := apiclient.New(srv.URL, "tkn")

	_, err := runBind(context.Background(), c, "github.com/acme/app", "unknown")
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("want 'unknown' error, got %v", err)
	}
}

// --- TestRunUnbind ---

func TestRunUnbind_Success(t *testing.T) {
	srv, _, deletedQ := newBindSrv(t, nil, nil)
	c := apiclient.New(srv.URL, "tkn")

	out, err := runUnbind(context.Background(), c, "github.com/acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*deletedQ, "github.com%2Facme%2Fapp") && !strings.Contains(*deletedQ, "github.com/acme/app") {
		t.Errorf("DELETE query = %q, want remoteSlug present", *deletedQ)
	}
	if !strings.Contains(out, "github.com/acme/app") {
		t.Errorf("output missing origin slug: %q", out)
	}
}

// --- TestRunBindings ---

// makeGitRepo creates a temp dir, inits a git repo, and adds a remote origin.
// Returns the dir path. Skips the test if git is not available.
func makeGitRepo(t *testing.T, remoteURL string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("git", "init")
	run("git", "remote", "add", "origin", remoteURL)
	return dir
}

// TestRunBindings_MarksResolved: no FLOW_PROJECT override; cwd is a git repo whose
// origin resolves (via /projects/resolve) to project p1 → that binding is starred.
func TestRunBindings_MarksResolved(t *testing.T) {
	resolvedProj := &domain.Project{ID: "p1", Name: "Acme", Slug: "acme"}
	projects := []domain.Project{*resolvedProj}
	bindings := []domain.ProjectBinding{
		{ID: "b1", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/acme/app"},
		{ID: "b2", ProjectID: "p2", Kind: domain.BindingRemote, RemoteSlug: "github.com/acme/other"},
	}
	srv, _, _, _ := newBindSrvWithResolve(t, projects, bindings, resolvedProj)
	c := apiclient.New(srv.URL, "tkn")

	// Create a real git repo so OriginSlug returns a non-empty slug, causing /resolve to be called.
	dir := makeGitRepo(t, "https://github.com/acme/app.git")

	getenv := func(string) string { return "" }
	out, err := runBindings(context.Background(), c, getenv, dir)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var markedLine, unmarkedLine string
	for _, l := range lines {
		if strings.Contains(l, "github.com/acme/app") {
			markedLine = l
		}
		if strings.Contains(l, "github.com/acme/other") {
			unmarkedLine = l
		}
	}
	if !strings.Contains(markedLine, "*") {
		t.Errorf("resolved binding line should be marked with *:\n%s", out)
	}
	if strings.Contains(unmarkedLine, "*") {
		t.Errorf("non-resolved binding line should NOT be marked with *:\n%s", out)
	}
}

// TestRunBindings_FlowProjectOverride: FLOW_PROJECT=<slug> wins over git-remote;
// the binding for that project is starred even if cwd's origin would resolve differently.
func TestRunBindings_FlowProjectOverride(t *testing.T) {
	// Two projects; p2 is the override target, p1 is the cwd-origin match (if resolution went by remote).
	overrideProj := &domain.Project{ID: "p2", Name: "Override", Slug: "override-slug"}
	projects := []domain.Project{
		{ID: "p1", Name: "Acme", Slug: "acme"},
		*overrideProj,
	}
	bindings := []domain.ProjectBinding{
		{ID: "b1", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/acme/app"},
		{ID: "b2", ProjectID: "p2", Kind: domain.BindingRemote, RemoteSlug: "github.com/other/repo"},
	}
	// /resolve is not expected to be called (env override takes precedence before git-remote tier).
	srv, _, _, resolveHit := newBindSrvWithResolve(t, projects, bindings, nil)
	c := apiclient.New(srv.URL, "tkn")

	// getenv returns FLOW_PROJECT=override-slug; cwd can be anything (even /tmp — no git needed).
	getenv := func(k string) string {
		if k == "FLOW_PROJECT" {
			return "override-slug"
		}
		return ""
	}
	out, err := runBindings(context.Background(), c, getenv, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var overrideLine, otherLine string
	for _, l := range lines {
		if strings.Contains(l, "github.com/other/repo") {
			overrideLine = l
		}
		if strings.Contains(l, "github.com/acme/app") {
			otherLine = l
		}
	}
	if !strings.Contains(overrideLine, "*") {
		t.Errorf("FLOW_PROJECT override binding should be starred:\n%s", out)
	}
	if strings.Contains(otherLine, "*") {
		t.Errorf("cwd-origin binding should NOT be starred when override wins:\n%s", out)
	}
	if *resolveHit {
		t.Error("/projects/resolve should NOT have been called when FLOW_PROJECT is set")
	}
}

// TestRunBindings_NoOrigin_ListsAnyway: no env override, not in a git repo →
// resolution yields ok=false → no star, bindings still listed.
func TestRunBindings_NoOrigin_ListsAnyway(t *testing.T) {
	bindings := []domain.ProjectBinding{
		{ID: "b1", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"},
	}
	// /resolve will be called (empty slug from non-git dir) but returns 404.
	srv, _, _, _ := newBindSrvWithResolve(t, nil, bindings, nil)
	c := apiclient.New(srv.URL, "tkn")

	// Use a non-git temp dir so OriginSlug returns empty.
	getenv := func(string) string { return "" }
	out, err := runBindings(context.Background(), c, getenv, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "github.com/x/y") {
		t.Errorf("expected binding in output: %q", out)
	}
	if strings.Contains(out, "*") {
		t.Errorf("no resolved binding so * should not appear: %q", out)
	}
}

// --- TestRepoName ---

func TestRepoName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		slug string
		want string
	}{
		{"serverkraken/flow", "flow"},
		{"github.com/acme/app", "app"},
		{"github.com/acme/myrepo.git", "myrepo"},
		{"justname", "justname"},
		{"", "."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.slug, func(t *testing.T) {
			t.Parallel()
			if got := repoName(tc.slug); got != tc.want {
				t.Errorf("repoName(%q) = %q, want %q", tc.slug, got, tc.want)
			}
		})
	}
}

// --- TestPickProjectProgram ---

func testItems() []fuzzylist.Item {
	return []fuzzylist.Item{
		{ID: "p1", Label: "alpha"},
		{ID: "p2", Label: "beta"},
		{ID: "p3", Label: "gamma"},
	}
}

func charKey(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: s} }
func downKey() tea.KeyPressMsg         { return tea.KeyPressMsg{Code: tea.KeyDown} }
func enterKey() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func escKey() tea.KeyPressMsg          { return tea.KeyPressMsg{Code: tea.KeyEsc} }

func TestPickerProgram_SelectExisting(t *testing.T) {
	t.Parallel()
	m := newPickProjectProgram(testItems(), "", theme.Default)
	// Navigate to second item (beta) and press Enter.
	_, _ = m.Update(downKey())
	_, _ = m.Update(enterKey())
	if !m.result.ok || m.result.cancelled || m.result.isCreate {
		t.Fatalf("expected ok=true isCreate=false, got %+v", m.result)
	}
	if m.result.item.ID != "p2" {
		t.Errorf("expected p2 (beta), got %q", m.result.item.ID)
	}
}

func TestPickerProgram_SelectCreate(t *testing.T) {
	t.Parallel()
	m := newPickProjectProgram(testItems(), "", theme.Default)
	// Type a name that doesn't match any existing item.
	for _, ch := range "newproject" {
		_, _ = m.Update(charKey(string(ch)))
	}
	// The create row appears at the bottom. Move cursor all the way down.
	for i := 0; i < 10; i++ {
		_, _ = m.Update(downKey())
	}
	_, _ = m.Update(enterKey())

	if !m.result.ok || m.result.cancelled || !m.result.isCreate {
		t.Fatalf("expected isCreate=true, got %+v", m.result)
	}
	if m.result.item.Label != "newproject" {
		t.Errorf("create label = %q, want %q", m.result.item.Label, "newproject")
	}
}

func TestPickerProgram_DefaultNamePreFilled(t *testing.T) {
	t.Parallel()
	// defaultName "myrepo" should appear as the query immediately.
	m := newPickProjectProgram(testItems(), "myrepo", theme.Default)
	if m.list.Query() != "myrepo" {
		t.Errorf("query = %q, want myrepo (default name not pre-filled)", m.list.Query())
	}
}

func TestPickerProgram_CancelOnEsc(t *testing.T) {
	t.Parallel()
	m := newPickProjectProgram(testItems(), "", theme.Default)
	_, _ = m.Update(escKey())
	if !m.result.cancelled {
		t.Errorf("expected cancelled=true after Esc")
	}
	_, isCreate, ok := m.Selection()
	if ok || isCreate {
		t.Errorf("cancelled picker should return ok=false, got ok=%v isCreate=%v", ok, isCreate)
	}
}

// newBindSelectionSrv is an httptest server that handles BindRemote (PUT) and
// optionally CreateProject (POST /api/v1/projects). It records what was called.
type bindSelectionCapture struct {
	putProjectID   string
	putRemoteSlug  string
	postCreateName string
	postCalled     bool
}

func newBindSelectionSrv(t *testing.T, createResponse domain.Project) (*httptest.Server, *bindSelectionCapture) {
	t.Helper()
	cap := &bindSelectionCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			cap.postCreateName, _ = body["name"].(string)
			cap.postCalled = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(createResponse)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bindings"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			cap.putRemoteSlug, _ = body["remoteSlug"].(string)
			// Extract project ID from path: /api/v1/projects/<id>/bindings
			trimmed := strings.TrimSuffix(r.URL.Path, "/bindings")
			cap.putProjectID = strings.TrimPrefix(trimmed, "/api/v1/projects/")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(domain.ProjectBinding{RemoteSlug: cap.putRemoteSlug})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// TestBindSelection_PickExisting: pick-existing path calls BindRemote with the
// picked project ID and origin slug; CreateProject is NOT called.
func TestBindSelection_PickExisting(t *testing.T) {
	t.Parallel()
	srv, cap := newBindSelectionSrv(t, domain.Project{})
	c := apiclient.New(srv.URL, "tkn")

	picked := fuzzylist.Item{ID: "p1", Label: "Alpha"}
	out, err := bindSelection(context.Background(), c, "github.com/acme/alpha", picked, false)
	if err != nil {
		t.Fatalf("bindSelection: %v", err)
	}
	if cap.postCalled {
		t.Error("CreateProject should NOT have been called for pick-existing")
	}
	if cap.putProjectID != "p1" {
		t.Errorf("PUT project ID = %q, want %q", cap.putProjectID, "p1")
	}
	if cap.putRemoteSlug != "github.com/acme/alpha" {
		t.Errorf("PUT remoteSlug = %q, want %q", cap.putRemoteSlug, "github.com/acme/alpha")
	}
	if !strings.Contains(out, "github.com/acme/alpha") || !strings.Contains(out, "Alpha") {
		t.Errorf("output = %q; want origin and project name", out)
	}
}

// TestBindSelection_CreateNew: create-new path calls CreateProject(name) first,
// then BindRemote with the server-assigned ID (not the empty item ID).
func TestBindSelection_CreateNew(t *testing.T) {
	t.Parallel()
	newProject := domain.Project{ID: "p-new", Name: "Brandnew"}
	srv, cap := newBindSelectionSrv(t, newProject)
	c := apiclient.New(srv.URL, "tkn")

	// isCreate=true: the item has no ID yet (server assigns it on create)
	picked := fuzzylist.Item{ID: "", Label: "Brandnew"}
	out, err := bindSelection(context.Background(), c, "github.com/acme/repo", picked, true)
	if err != nil {
		t.Fatalf("bindSelection: %v", err)
	}
	if !cap.postCalled {
		t.Error("CreateProject should have been called for create-new")
	}
	if cap.postCreateName != "Brandnew" {
		t.Errorf("CreateProject name = %q, want %q", cap.postCreateName, "Brandnew")
	}
	// BindRemote must use the server-assigned ID, not the empty item ID.
	if cap.putProjectID != "p-new" {
		t.Errorf("PUT project ID = %q, want server-assigned %q", cap.putProjectID, "p-new")
	}
	if cap.putRemoteSlug != "github.com/acme/repo" {
		t.Errorf("PUT remoteSlug = %q, want %q", cap.putRemoteSlug, "github.com/acme/repo")
	}
	if !strings.Contains(out, "github.com/acme/repo") || !strings.Contains(out, "Brandnew") {
		t.Errorf("output = %q; want origin and project name", out)
	}
}

// --- Path-binding helpers ---

// newPathBindSrv creates an httptest server that captures PUT/DELETE for path bindings.
// It records the PUT body fields and the DELETE query string.
type pathBindCapture struct {
	putProjectID string
	putKind      string
	putMachineID string
	putPath      string
	deleteQuery  string
}

func newPathBindSrv(t *testing.T) (*httptest.Server, *pathBindCapture) {
	t.Helper()
	cap := &pathBindCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bindings"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			cap.putKind, _ = body["kind"].(string)
			cap.putMachineID, _ = body["machineId"].(string)
			// extract project ID from /api/v1/projects/<id>/bindings
			trimmed := strings.TrimSuffix(r.URL.Path, "/bindings")
			cap.putProjectID = strings.TrimPrefix(trimmed, "/api/v1/projects/")
			cap.putPath, _ = body["path"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(domain.ProjectBinding{Kind: domain.BindingPath})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/bindings":
			cap.deleteQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// TestRunBindPath_Success: runBindPath calls BindPath with kind=path,
// the machine ID, and the cleaned cwd; returns a confirmation message.
func TestRunBindPath_Success(t *testing.T) {
	t.Parallel()
	srv, cap := newPathBindSrv(t)
	c := apiclient.New(srv.URL, "tkn")
	m := clientmachine.Machine{ID: "m-123", Label: "myhost"}
	cwd := "/home/user/projects/myapp"

	out, err := runBindPath(context.Background(), c, m, cwd, "proj-1", "MyApp")
	if err != nil {
		t.Fatalf("runBindPath: %v", err)
	}
	if cap.putKind != "path" {
		t.Errorf("PUT kind = %q, want %q", cap.putKind, "path")
	}
	if cap.putProjectID != "proj-1" {
		t.Errorf("PUT projectID = %q, want %q", cap.putProjectID, "proj-1")
	}
	if cap.putMachineID != "m-123" {
		t.Errorf("PUT machineID = %q, want %q", cap.putMachineID, "m-123")
	}
	if cap.putPath != cwd {
		t.Errorf("PUT path = %q, want %q", cap.putPath, cwd)
	}
	if !strings.Contains(out, cwd) {
		t.Errorf("output should contain cwd %q: %q", cwd, out)
	}
	if !strings.Contains(out, "myhost") {
		t.Errorf("output should contain machine label %q: %q", "myhost", out)
	}
	if !strings.Contains(out, "MyApp") {
		t.Errorf("output should contain project name %q: %q", "MyApp", out)
	}
}

// TestRunUnbindPath_Success: runUnbindPath calls UnbindPath with the machine ID
// and the cleaned cwd; returns a confirmation message.
func TestRunUnbindPath_Success(t *testing.T) {
	t.Parallel()
	srv, cap := newPathBindSrv(t)
	c := apiclient.New(srv.URL, "tkn")
	m := clientmachine.Machine{ID: "m-123", Label: "myhost"}
	cwd := "/home/user/projects/myapp"

	out, err := runUnbindPath(context.Background(), c, m, cwd)
	if err != nil {
		t.Fatalf("runUnbindPath: %v", err)
	}
	if !strings.Contains(cap.deleteQuery, "m-123") {
		t.Errorf("DELETE query should contain machine ID: %q", cap.deleteQuery)
	}
	if !strings.Contains(cap.deleteQuery, "kind=path") {
		t.Errorf("DELETE query should contain kind=path: %q", cap.deleteQuery)
	}
	if !strings.Contains(out, cwd) {
		t.Errorf("output should contain cwd %q: %q", cwd, out)
	}
	if !strings.Contains(out, "myhost") {
		t.Errorf("output should contain machine label %q: %q", "myhost", out)
	}
}
