package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
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

func newServer() *httpserver.Server {
	store := testutil.NewFakeUserStore()
	bus := sse.NewBus()
	return &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: func(id ports.Identity) bool { return id.Subject == "msoent" }},
		Bus:      bus,
		Emitter:  sse.NewEmitter(bus, &fakeActivityStore{}, &testutil.FakeIDGen{}, testutil.FakeClock{}),
		Dev:      true,
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health: %v status=%v", err, res.StatusCode)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, _ := http.Get(srv.URL + "/api/v1/me")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestMeReturnsUser(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("me: %v status=%v", err, res.StatusCode)
	}
	body := make([]byte, 256)
	n, _ := res.Body.Read(body)
	if !strings.Contains(string(body[:n]), `"username":"msoent"`) {
		t.Fatalf("unexpected body: %s", body[:n])
	}
}

func TestSessionStartStopRoutes(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Verifier:     testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:       usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:          bus,
		Emitter:      sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:        clk,
		StartSession: usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:  usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		ListSessions: usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateNode:   usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:    usecase.ListNodes{Nodes: ps},
		GetNode:      usecase.GetNode{Nodes: ps},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

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

	res := do("POST", "/api/v1/nodes", `{"name":"Flow","kind":"engagement"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project status %d", res.StatusCode)
	}
	var proj domain.Node
	_ = json.NewDecoder(res.Body).Decode(&proj)
	_ = res.Body.Close()

	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("start status %d", res.StatusCode)
	}
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	_ = res.Body.Close()

	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("double start status %d, want 409", res.StatusCode)
	}
	_ = res.Body.Close()

	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("stop-no-project status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{"projectId":"`+proj.ID+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop status %d, want 200", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestSessionEditDeleteRoutes(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	bus146 := sse.NewBus()
	srv := &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           bus146,
		Emitter:       sse.NewEmitter(bus146, &fakeActivityStore{}, ids, clk),
		Clock:         clk,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateNode:    usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:     usecase.ListNodes{Nodes: ps},
		GetNode:       usecase.GetNode{Nodes: ps},
		EditSession:   usecase.EditSession{Sessions: ss, Nodes: ps},
		DeleteSession: usecase.DeleteSession{Sessions: ss},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

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

	// start then stop (with a project) to get a completed session
	res := do("POST", "/api/v1/nodes", `{"name":"Flow","kind":"engagement"}`)
	var proj domain.Node
	_ = json.NewDecoder(res.Body).Decode(&proj)
	_ = res.Body.Close()
	res = do("POST", "/api/v1/sessions", `{}`)
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	_ = res.Body.Close()
	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{"projectId":"`+proj.ID+`"}`)
	_ = res.Body.Close()

	// PATCH edit: set a tag
	res = do("PATCH", "/api/v1/sessions/"+s.ID, `{"projectId":"`+proj.ID+`","tags":["deep"],"note":"","start":"2026-06-14T09:00:00Z","stop":"2026-06-14T11:00:00Z"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit status %d, want 200", res.StatusCode)
	}
	var edited domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&edited)
	_ = res.Body.Close()
	if len(edited.Tags) != 1 || edited.Tags[0] != "deep" {
		t.Fatalf("edit did not persist tags: %+v", edited)
	}

	// PATCH invalid times -> 400
	res = do("PATCH", "/api/v1/sessions/"+s.ID, `{"start":"2026-06-14T11:00:00Z","stop":"2026-06-14T09:00:00Z"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid edit status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// PATCH unknown id -> 404 (owner-scoping enforced by the store)
	res = do("PATCH", "/api/v1/sessions/does-not-exist", `{"start":"2026-06-14T09:00:00Z","stop":"2026-06-14T11:00:00Z"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown-id edit status %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()

	// DELETE -> 204, then 404
	res = do("DELETE", "/api/v1/sessions/"+s.ID, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d, want 204", res.StatusCode)
	}
	_ = res.Body.Close()
	res = do("DELETE", "/api/v1/sessions/"+s.ID, "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("double delete status %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestListSessionsAndProjects(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier:     testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:       usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:          sse.NewBus(),
		Clock:        clk,
		StartSession: usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:  usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		ListSessions: usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateNode:   usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:    usecase.ListNodes{Nodes: ps},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

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

	res := do("GET", "/api/v1/sessions", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list sessions: %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = do("GET", "/api/v1/nodes", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list projects: %d", res.StatusCode)
	}
	_ = res.Body.Close()

	// since param
	since := clk.T.Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res = do("GET", "/api/v1/sessions?since="+since, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list sessions with since: %d", res.StatusCode)
	}
	_ = res.Body.Close()
}

// newWebSrv builds a full httpserver.Server with a seeded user and returns the
// test server, the session codec, and the user id.
func newWebSrv(t *testing.T) (*httptest.Server, *websession.Codec, string) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()

	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)

	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	webBus := sse.NewBus()
	srv := &httpserver.Server{
		Ensure:            usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               webBus,
		Emitter:           sse.NewEmitter(webBus, &fakeActivityStore{}, ids, clk),
		Clock:             clk,
		Users:             users,
		Session:           codec,
		OIDCAuth:          fakeAuth{url: "https://id/authorize?state="},
		StartSession:      usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:       usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		ListSessions:      usecase.ListSessions{Sessions: ss, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		CreateNode:        usecase.CreateNode{Nodes: ps, IDs: ids, Clock: clk},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, codec, "u1"
}

func TestWebFragmentRequiresSession(t *testing.T) {
	ts, _, _ := newWebSrv(t)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noFollow.Get(ts.URL + "/ui/worktime")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", res.StatusCode)
	}
}

func TestWebFragmentWithSession(t *testing.T) {
	ts, codec, uid := newWebSrv(t)
	cookieVal, _ := codec.Issue(uid)
	req, _ := http.NewRequest("GET", ts.URL+"/ui/worktime", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("fragment status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	// Heute is a pure glass ledger since Kristall K3 — the idle state shows the
	// Nachbuchen add SessionDialog, not a start form (owned by the sidebar widget).
	if !strings.Contains(string(body), "/ui/worktime/add") {
		t.Fatalf("fragment missing add SessionDialog: %s", string(body))
	}
}

// Session start/stop over HTTP is now covered end-to-end by
// TestTimerWidget_Lifecycle (webui_timer_test.go) against the K1 shell timer
// widget's /ui/timer/start|stop routes — the sole start/stop surface since
// K3 Task 6 retired /ui/worktime/start|stop.
