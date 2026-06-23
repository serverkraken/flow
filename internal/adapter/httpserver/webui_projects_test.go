package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// getWeb is an alias for getWebProjects, used by cockpit and future webui tests.
func getWeb(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) (int, string) {
	t.Helper()
	return getWebProjects(t, ts, c, path)
}

// newWebProjectsServer builds a webui-capable server with the project usecases
// wired and a seeded user; returns the test server, a session cookie, and the
// fake project store for seeding.
func newWebProjectsServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeProjectStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)

	ss := testutil.NewFakeSessionStore()
	docs := testutil.NewFakeDocumentStore()
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     sse.NewBus(),
		Clock:   clk,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   ids,
			Allow: func(ports.Identity) bool { return true },
		},
		CreateProject:       usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:        usecase.ListProjects{Projects: ps},
		GetProject:          usecase.GetProject{Projects: ps},
		UpdateProject:       usecase.UpdateProject{Projects: ps, Bindings: bs, IDs: ids, Clock: clk},
		DeleteProject:       usecase.DeleteProject{Projects: ps},
		SetProjectRate:      usecase.SetProjectRate{Projects: ps},
		ListSessionsRange:   usecase.ListSessionsRange{Sessions: ss},
		ListProjectBindings: usecase.ListProjectBindings{Bindings: bs},
		ListDocuments:       usecase.ListDocuments{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookieVal, _ := codec.Issue("u1")
	cookie := &http.Cookie{Name: "flow_session", Value: cookieVal}
	return ts, cookie, ps
}

func seedProjectForWeb(t *testing.T, ps *testutil.FakeProjectStore, id, name string, status domain.ProjectStatus) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, err := domain.NewProject(id, "u1", name, name, now)
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	p.Status = status
	_, _ = ps.Create(context.Background(), p)
}

func getWebProjects(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.AddCookie(c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var b []byte
	buf := make([]byte, 4096)
	for {
		n, e := res.Body.Read(buf)
		b = append(b, buf[:n]...)
		if e != nil {
			break
		}
	}
	return res.StatusCode, string(b)
}

func TestWebProjectCockpit(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	p.Description = "# Notiz\nhallo"
	p.UpstreamGit = "git@github.com:serverkraken/flow.git"
	p.Status = domain.ProjectPaused
	p.Color = domain.ProjectColors[0]
	_, _ = ps.Create(context.Background(), p)

	code, body := getWeb(t, ts, c, "/projects/p1")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{"Flow", "pausiert", "github.com/serverkraken/flow", "Bearbeiten"} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q", want)
		}
	}
	// rendered markdown description (goldmark → <h1>)
	if !strings.Contains(body, "Notiz") {
		t.Errorf("description should render")
	}
	// unknown id → 404
	code404, _ := getWeb(t, ts, c, "/projects/nope")
	if code404 != http.StatusNotFound {
		t.Errorf("unknown id status %d, want 404", code404)
	}
}

func TestWebProjectsListAndFilter(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	seedProjectForWeb(t, ps, "p1", "Aaa", domain.ProjectActive)
	seedProjectForWeb(t, ps, "p2", "Bbb", domain.ProjectPaused)
	seedProjectForWeb(t, ps, "p3", "Ccc", domain.ProjectArchived)

	// default: active+paused → Aaa, Bbb shown, Ccc (archived) hidden
	code, body := getWebProjects(t, ts, c, "/projects")
	if code != 200 {
		t.Fatalf("GET /projects status %d body=%.300s", code, body)
	}
	if !strings.Contains(body, "Aaa") || !strings.Contains(body, "Bbb") {
		t.Errorf("default should list active+paused; body=%.400s", body)
	}
	if strings.Contains(body, "Ccc") {
		t.Errorf("default must hide archived; body=%.400s", body)
	}
	// archived filter reveals Ccc
	_, arch := getWebProjects(t, ts, c, "/projects?status=archived")
	if !strings.Contains(arch, "Ccc") {
		t.Errorf("archived filter should show Ccc; body=%.400s", arch)
	}
	// status badge label present for paused
	if !strings.Contains(body, "pausiert") {
		t.Errorf("paused badge label expected; body=%.400s", body)
	}
	// SSE fragment route works
	codeF, _ := getWebProjects(t, ts, c, "/ui/projects/list")
	if codeF != 200 {
		t.Errorf("GET /ui/projects/list status %d", codeF)
	}
}
