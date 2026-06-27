package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
func newWebProjectsServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ps := testutil.NewFakeNodeStore()
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
		CreateNode:       usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:        usecase.ListNodes{Nodes: ps},
		GetNode:          usecase.GetNode{Nodes: ps},
		UpdateNode:       usecase.UpdateNode{Nodes: ps, Bindings: bs, IDs: ids, Clock: clk},
		DeleteNode:       usecase.DeleteNode{Nodes: ps},
		SetNodeRate:      usecase.SetNodeRate{Nodes: ps},
		ListSessionsRange:   usecase.ListSessionsRange{Sessions: ss},
		ListNodeBindings: usecase.ListNodeBindings{Bindings: bs},
		ListDocuments:       usecase.ListDocuments{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookieVal, _ := codec.Issue("u1")
	cookie := &http.Cookie{Name: "flow_session", Value: cookieVal}
	return ts, cookie, ps
}

func seedProjectForWeb(t *testing.T, ps *testutil.FakeNodeStore, id, name string, status domain.NodeStatus) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, err := domain.NewNode(id, "u1", name, name, now)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
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

func postWebForm(t *testing.T, ts *httptest.Server, c *http.Cookie, path string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(form.Encode()))
	req.AddCookie(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebProjectCreateEditStatusDelete(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)

	// CREATE with upstream + color + rate
	res := postWebForm(t, ts, c, "/nodes", url.Values{
		"name": {"PM Web"}, "slug": {"pm-web"}, "description": {"# Hi"},
		"upstreamGit": {"git@github.com:serverkraken/pmweb.git"}, "status": {"active"},
		"color": {domain.NodeColors[0]}, "glyph": {domain.NodeGlyphs[0]},
		"rateAmount": {"90.00"}, "rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.HasPrefix(loc, "/nodes/") {
		t.Fatalf("create redirect = %q", loc)
	}
	id := strings.TrimPrefix(loc, "/nodes/")

	// the cockpit reflects the saved fields + rate earnings
	_, body := getWeb(t, ts, c, "/nodes/"+id)
	if !strings.Contains(body, "PM Web") || !strings.Contains(body, "github.com/serverkraken/pmweb") {
		t.Errorf("created project not reflected: %s", body)
	}

	// EDIT → pause + change description
	res = postWebForm(t, ts, c, "/nodes/"+id, url.Values{
		"name": {"PM Web"}, "slug": {"pm-web"}, "description": {"changed"},
		"upstreamGit": {"git@github.com:serverkraken/pmweb.git"}, "status": {"paused"},
		"color": {domain.NodeColors[0]}, "glyph": {""}, "rateAmount": {""}, "rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit status %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	p, _ := ps.Get(context.Background(), "u1", id)
	if p.Status != domain.NodePaused {
		t.Errorf("edit did not pause: %s", p.Status)
	}
	if p.Rate != nil {
		t.Errorf("blank rateAmount must clear the rate, got %+v", p.Rate)
	}

	// REGRESSION GUARD: the delete <form> must NOT be nested inside the outer edit
	// <form>. HTML5 reparents a nested form's button to the outer form, which would
	// silently turn "Löschen" into an UPDATE submit. Assert a </form> closes the
	// outer edit form BEFORE the delete-form action appears.
	_, edit := getWeb(t, ts, c, "/nodes/"+id+"/edit")
	outerIdx := strings.Index(edit, `action="/nodes/`+id+`"`)
	delIdx := strings.Index(edit, `action="/nodes/`+id+`/delete"`)
	if outerIdx < 0 || delIdx < 0 {
		t.Fatalf("edit form missing outer (%d) or delete (%d) action; body=%.600s", outerIdx, delIdx, edit)
	}
	if delIdx < outerIdx {
		t.Fatalf("delete action appears before the outer edit-form action")
	}
	if !strings.Contains(edit[outerIdx:delIdx], "</form>") {
		t.Errorf("delete form is nested inside the outer edit form (no </form> between the two actions)")
	}

	// STATUS action → archive
	res = postWebForm(t, ts, c, "/nodes/"+id+"/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status action %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	p, _ = ps.Get(context.Background(), "u1", id)
	if p.Status != domain.NodeArchived {
		t.Errorf("status action did not archive: %s", p.Status)
	}

	// CREATE with bad upstream → 400 + re-rendered form
	before, _ := ps.List(context.Background(), "u1")
	res = postWebForm(t, ts, c, "/nodes", url.Values{"name": {"Bad"}, "upstreamGit": {"garbage"}, "status": {"active"}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad upstream status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()
	// Up-front upstream validation must reject BEFORE CreateNode, so no orphan
	// name-only project is left behind.
	after, _ := ps.List(context.Background(), "u1")
	if len(after) != len(before) {
		t.Errorf("bad upstream created an orphan project: count %d → %d", len(before), len(after))
	}
	for _, pr := range after {
		if pr.Name == "Bad" {
			t.Errorf("bad upstream left orphan project %q (id=%s)", pr.Name, pr.ID)
		}
	}

	// DELETE
	res = postWebForm(t, ts, c, "/nodes/"+id+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete status %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, err := ps.Get(context.Background(), "u1", id); err == nil {
		t.Errorf("project should be deleted")
	}
}

func TestWebProjectCockpit(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewNode("p1", "u1", "Flow", "flow", now)
	p.Description = "# Notiz\nhallo"
	p.UpstreamGit = "git@github.com:serverkraken/flow.git"
	p.Status = domain.NodePaused
	p.Color = domain.NodeColors[0]
	_, _ = ps.Create(context.Background(), p)

	code, body := getWeb(t, ts, c, "/nodes/p1")
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
	code404, _ := getWeb(t, ts, c, "/nodes/nope")
	if code404 != http.StatusNotFound {
		t.Errorf("unknown id status %d, want 404", code404)
	}
}

func TestWebProjectsListAndFilter(t *testing.T) {
	ts, c, ps := newWebProjectsServer(t)
	seedProjectForWeb(t, ps, "p1", "Aaa", domain.NodeActive)
	seedProjectForWeb(t, ps, "p2", "Bbb", domain.NodePaused)
	seedProjectForWeb(t, ps, "p3", "Ccc", domain.NodeArchived)

	// default: active+paused → Aaa, Bbb shown, Ccc (archived) hidden
	code, body := getWebProjects(t, ts, c, "/nodes")
	if code != 200 {
		t.Fatalf("GET /nodes status %d body=%.300s", code, body)
	}
	if !strings.Contains(body, "Aaa") || !strings.Contains(body, "Bbb") {
		t.Errorf("default should list active+paused; body=%.400s", body)
	}
	if strings.Contains(body, "Ccc") {
		t.Errorf("default must hide archived; body=%.400s", body)
	}
	// archived filter reveals Ccc
	_, arch := getWebProjects(t, ts, c, "/nodes?status=archived")
	if !strings.Contains(arch, "Ccc") {
		t.Errorf("archived filter should show Ccc; body=%.400s", arch)
	}
	// status badge label present for paused
	if !strings.Contains(body, "pausiert") {
		t.Errorf("paused badge label expected; body=%.400s", body)
	}
	// SSE fragment route works
	codeF, _ := getWebProjects(t, ts, c, "/ui/nodes/list")
	if codeF != 200 {
		t.Errorf("GET /ui/nodes/list status %d", codeF)
	}
}
