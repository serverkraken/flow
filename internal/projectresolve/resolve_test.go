package projectresolve_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/projectresolve"
)

// projectJSON returns a JSON-encoded Project with the given slug and id.
func projectJSON(id, name, slug string) string {
	return `{"id":"` + id + `","name":"` + name + `","slug":"` + slug + `","status":"active"}`
}

// newTestServer builds an httptest server with:
//   - GET /api/v1/projects → returns projects list
//   - GET /api/v1/projects/resolve → returns resolveProject (404 if nil) and
//     sets *resolveHit=true so tests can assert whether /resolve was called.
func newTestServer(t *testing.T, projects []map[string]string, resolveSlug string, resolveHit *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects" && r.URL.RawQuery == "":
			var list []map[string]string
			if projects != nil {
				list = projects
			}
			_ = json.NewEncoder(w).Encode(list)

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/resolve":
			if resolveHit != nil {
				*resolveHit = true
			}
			if resolveSlug == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(projectJSON("p2", "FlowRepo", resolveSlug)))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

// TestResolve_EnvOverride: FLOW_PROJECT=flow → ListProjects, returns slug match, /resolve NOT called.
func TestResolve_EnvOverride(t *testing.T) {
	var resolveHit bool
	projects := []map[string]string{
		{"id": "p1", "name": "Flow", "slug": "flow", "status": "active"},
		{"id": "p3", "name": "Other", "slug": "other", "status": "active"},
	}
	ts := newTestServer(t, projects, "", &resolveHit)
	defer ts.Close()

	c := apiclient.New(ts.URL, "tkn")
	getenv := func(k string) string {
		if k == "FLOW_PROJECT" {
			return "flow"
		}
		return ""
	}

	p, ok, err := projectresolve.Resolve(context.Background(), c, getenv, "/tmp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Slug != "flow" || p.ID != "p1" {
		t.Fatalf("project = %+v", p)
	}
	if resolveHit {
		t.Fatal("/resolve should NOT have been called when FLOW_PROJECT is set")
	}
}

// TestResolve_GitRemote: FLOW_PROJECT unset + cwd is a git repo → /resolve IS called with remote slug.
func TestResolve_GitRemote(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a temp git repo with a remote.
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("git", "init")
	run("git", "remote", "add", "origin", "https://github.com/serverkraken/flow-rebuild.git")

	var resolveHit bool
	var gotSlug string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/resolve":
			resolveHit = true
			gotSlug = r.URL.Query().Get("slug")
			_, _ = w.Write([]byte(projectJSON("p2", "FlowRebuild", gotSlug)))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tkn")
	getenv := func(string) string { return "" }

	p, ok, err := projectresolve.Resolve(context.Background(), c, getenv, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !resolveHit {
		t.Fatal("/resolve should have been called")
	}
	// The remote slug for github.com/serverkraken/flow-rebuild should be non-empty.
	if gotSlug == "" {
		t.Fatal("expected non-empty slug forwarded to /resolve")
	}
	_ = p
}

// TestResolve_UnknownEnvSlug: FLOW_PROJECT set to unknown slug → error, no panic.
func TestResolve_UnknownEnvSlug(t *testing.T) {
	projects := []map[string]string{
		{"id": "p1", "name": "Flow", "slug": "flow", "status": "active"},
	}
	ts := newTestServer(t, projects, "", nil)
	defer ts.Close()

	c := apiclient.New(ts.URL, "tkn")
	getenv := func(k string) string {
		if k == "FLOW_PROJECT" {
			return "nope"
		}
		return ""
	}

	_, ok, err := projectresolve.Resolve(context.Background(), c, getenv, "/tmp")
	if err == nil {
		t.Fatal("expected error for unknown slug")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
	if !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("error message should mention the slug, got: %v", err)
	}
}

// TestResolve_OutsideGitRepo: no env, not a git repo → /resolve called with empty slug (server returns 404 → ok=false).
func TestResolve_OutsideGitRepo(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "not-a-repo-"+t.Name())
	_ = os.MkdirAll(dir, 0o700)
	defer func() { _ = os.RemoveAll(dir) }()

	var resolveHit bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/resolve" {
			resolveHit = true
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tkn")
	getenv := func(string) string { return "" }

	_, ok, err := projectresolve.Resolve(context.Background(), c, getenv, dir)
	if err != nil {
		t.Fatalf("expected nil error for 404 resolve, got: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when server returns 404")
	}
	if !resolveHit {
		t.Fatal("/resolve should have been called")
	}
}
