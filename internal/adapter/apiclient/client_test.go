package apiclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestWhoamiSendsBearerAndParses(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"u1","username":"msoent","email":"m@x.de","displayName":"Martin"}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok-123")
	u, err := c.Whoami(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	if u.Username != "msoent" {
		t.Fatalf("parse: %+v", u)
	}
}

func TestWhoamiNon200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	_, err := c.Whoami(t.Context())
	if err == nil {
		t.Fatal("expected error on non-200")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should mention status: %v", err)
	}
}

func newMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return mux, srv.URL
}

func TestStopSession(t *testing.T) {
	mux, base := newMux(t)
	stop := time.Now().UTC().Truncate(time.Second)
	mux.HandleFunc("POST /api/v1/sessions/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ws, _ := domain.NewWorkSession(id, "u1", nil, stop.Add(-time.Hour))
		ws.Stop = &stop
		_ = json.NewEncoder(w).Encode(ws)
	})
	c := apiclient.New(base, "tok")
	ws, err := c.StopSession(t.Context(), "sess-1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID != "sess-1" {
		t.Fatalf("unexpected session id: %s", ws.ID)
	}
}

func TestListSessions(t *testing.T) {
	mux, base := newMux(t)
	start := time.Now().UTC().Truncate(time.Second)
	mux.HandleFunc("GET /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		ws, _ := domain.NewWorkSession("s1", "u1", nil, start)
		_ = json.NewEncoder(w).Encode([]domain.WorkSession{ws})
	})
	c := apiclient.New(base, "tok")
	list, err := c.ListSessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v", list)
	}
}

func TestCreateProject(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("POST /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		p, _ := domain.NewNode("p1", "u1", "Flow", "flow", time.Now())
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	})
	c := apiclient.New(base, "tok")
	p, err := c.CreateNode(t.Context(), "Flow")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Flow" {
		t.Fatalf("unexpected project: %+v", p)
	}
}

func TestDoErrorOnNon2xx(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("GET /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := apiclient.New(base, "tok")
	_, err := c.ListSessions(t.Context())
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

type tagRoundTripper struct{ tag string }

func (rt tagRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+rt.tag)
	return http.DefaultTransport.RoundTrip(r2)
}

func TestNewTransportSetsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"u1","username":"msoent"}`))
	}))
	defer srv.Close()

	c := apiclient.NewTransport(srv.URL, tagRoundTripper{tag: "from-rt"})
	if _, err := c.Whoami(t.Context()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer from-rt" {
		t.Fatalf("auth header: %q", gotAuth)
	}
}

func TestClientUpdateAndGetProject(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_ = json.NewEncoder(w).Encode(domain.Node{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.NodePaused, UpstreamGit: "git@github.com:serverkraken/flow.git"})
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	got, err := c.UpdateNode(context.Background(), "p1", apiclient.UpdateNodeFields{
		Name: "Flow", Slug: "flow", Status: "paused", UpstreamGit: "git@github.com:serverkraken/flow.git",
	})
	if err != nil || got.Status != domain.NodePaused {
		t.Fatalf("UpdateNode: %+v err=%v", got, err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/nodes/p1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"status":"paused"`) {
		t.Errorf("body missing status: %s", gotBody)
	}

	one, err := c.GetNode(context.Background(), "p1")
	if err != nil || one.Slug != "flow" {
		t.Fatalf("GetNode: %+v err=%v", one, err)
	}
	if gotMethod != "GET" || gotPath != "/api/v1/nodes/p1" {
		t.Errorf("GET method/path = %s %s", gotMethod, gotPath)
	}
}
