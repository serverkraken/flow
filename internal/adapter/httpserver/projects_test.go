package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func newProjectsSrv(t *testing.T) (*httptest.Server, func(method, path, body string) *http.Response) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()

	srv := &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
		DeleteProject: usecase.DeleteProject{Projects: ps},
	}

	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	return ts, do
}

func TestDeleteProject_204AndGone(t *testing.T) {
	_, do := newProjectsSrv(t)

	// Create a project first.
	res := do("POST", "/api/v1/projects", `{"name":"Testprojekt"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project status %d, want 201", res.StatusCode)
	}
	var p domain.Project
	if err := decodeJSON(res.Body, &p); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	_ = res.Body.Close()

	// DELETE the project → 204.
	res = do("DELETE", "/api/v1/projects/"+p.ID, "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d, want 204", res.StatusCode)
	}

	// GET the list → project must be gone.
	res = do("GET", "/api/v1/projects", "")
	var list []domain.Project
	if err := decodeJSON(res.Body, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	_ = res.Body.Close()
	for _, proj := range list {
		if proj.ID == p.ID {
			t.Fatalf("deleted project still in list: %+v", proj)
		}
	}
}

func TestDeleteProject_UnknownID404(t *testing.T) {
	_, do := newProjectsSrv(t)
	res := do("DELETE", "/api/v1/projects/does-not-exist", "")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id status %d, want 404", res.StatusCode)
	}
}
