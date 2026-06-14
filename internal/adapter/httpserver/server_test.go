package httpserver_test

import (
	"encoding/json"
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

func newServer() *httpserver.Server {
	store := testutil.NewFakeUserStore()
	return &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: func(id ports.Identity) bool { return id.Subject == "msoent" }},
		Bus:      sse.NewBus(),
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
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(),
		Clock:    clk,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
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

	res := do("POST", "/api/v1/projects", `{"name":"Flow"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project status %d", res.StatusCode)
	}
	var proj domain.Project
	_ = json.NewDecoder(res.Body).Decode(&proj)
	res.Body.Close()

	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("start status %d", res.StatusCode)
	}
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	res.Body.Close()

	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("double start status %d, want 409", res.StatusCode)
	}
	res.Body.Close()

	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("stop-no-project status %d, want 400", res.StatusCode)
	}
	res.Body.Close()

	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{"projectId":"`+proj.ID+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop status %d, want 200", res.StatusCode)
	}
	res.Body.Close()
}
