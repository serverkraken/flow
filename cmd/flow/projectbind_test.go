package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
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
