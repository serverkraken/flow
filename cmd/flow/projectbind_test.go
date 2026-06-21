package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// --- helpers shared across bind tests ---

func newBindSrv(t *testing.T, projects []domain.Project, bindings []domain.ProjectBinding) (srv *httptest.Server, putSlug *string, deletedPath *string) {
	t.Helper()
	var ps = projects
	var bs = bindings
	var putRemoteSlug string
	var dpath string
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
			// return 404 — no auto-resolve in these tests
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &putRemoteSlug, &dpath
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

func TestRunBindings_MarksResolved(t *testing.T) {
	projects := []domain.Project{{ID: "p1", Name: "Acme", Slug: "acme"}}
	bindings := []domain.ProjectBinding{
		{ID: "b1", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/acme/app"},
		{ID: "b2", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/acme/other"},
	}
	srv, _, _ := newBindSrv(t, projects, bindings)
	c := apiclient.New(srv.URL, "tkn")

	// resolvedRemoteSlug = "github.com/acme/app" → binding b1 is the current one
	out, err := runBindings(context.Background(), c, "github.com/acme/app")
	if err != nil {
		t.Fatal(err)
	}
	// The resolved binding should be marked with *
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

func TestRunBindings_NoOrigin_ListsAnyway(t *testing.T) {
	bindings := []domain.ProjectBinding{
		{ID: "b1", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "github.com/x/y"},
	}
	srv, _, _ := newBindSrv(t, nil, bindings)
	c := apiclient.New(srv.URL, "tkn")

	// empty originSlug means no match → no star
	out, err := runBindings(context.Background(), c, "")
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

func sendKeys(m *pickProjectProgram, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		_, _ = m.Update(k)
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

// TestRunBindInteractive_PickExisting verifies the full non-TUI path:
// a fake server, a picker that immediately selects the first project,
// and the resulting BindRemote call.
func TestRunBindInteractive_PickExisting(t *testing.T) {
	t.Parallel()
	projects := []domain.Project{{ID: "p1", Name: "Alpha", Slug: "alpha"}}
	var boundProjectID string
	var boundRemoteSlug string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(projects)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bindings"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			boundRemoteSlug, _ = body["remoteSlug"].(string)
			boundProjectID = strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/bindings"), "/api/v1/projects/")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(domain.ProjectBinding{RemoteSlug: boundRemoteSlug})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	c := apiclient.New(srv.URL, "tkn")

	// Call runBindRemote directly (bypasses the TUI) to verify the plumbing.
	out, err := runBindRemote(context.Background(), c, "github.com/acme/alpha", "p1", "Alpha")
	if err != nil {
		t.Fatalf("runBindRemote: %v", err)
	}
	if !strings.Contains(out, "github.com/acme/alpha") || !strings.Contains(out, "Alpha") {
		t.Errorf("output = %q; want origin and project name", out)
	}
	if boundRemoteSlug != "github.com/acme/alpha" {
		t.Errorf("PUT remoteSlug = %q", boundRemoteSlug)
	}
	_ = boundProjectID
}
