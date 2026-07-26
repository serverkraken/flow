package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeInspectBackend serves the five reads flow_get_node performs.
// Tree: e1/alpha → v1/rebuild → r1/jukebox.
func fakeInspectBackend(t *testing.T) *httptest.Server {
	t.Helper()
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive,
			Description: "der Plattenspieler"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/ancestors", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "r1" {
			_ = json.NewEncoder(w).Encode([]domain.Node{})
			return
		}
		_ = json.NewEncoder(w).Encode([]domain.Node{nodes[2], nodes[1], nodes[0]}) // leaf→root
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Tag{{ID: "t1", Slug: "go", Display: "go"}})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300})
	})
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		// Owner-wide across ALL devices — including one that belongs to another node.
		_ = json.NewEncoder(w).Encode([]domain.ProjectBinding{
			{ID: "b1", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
			{ID: "b2", NodeID: "e1", Kind: domain.BindingPath, MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"},
		})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range nodes {
			if n.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

// authedInspectServer binds the session to r1/jukebox so the omitted-node path
// is exercisable; unbound is a separate helper.
func authedInspectServer(t *testing.T, bound bool) *mcp.ClientSession {
	t.Helper()
	be := fakeInspectBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) { return client, nil }, nil)
	_, h := newServerH(mgr)
	mgr.onAuth = nil
	if bound {
		h.projMu.Lock()
		h.proj, h.matched = proj, true
		h.projMu.Unlock()
	}
	return connect(t, h.srv)
}

func TestLoopback_GetNode_Advertised(t *testing.T) {
	sess := authedInspectServer(t, true)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_get_node") {
		t.Fatalf("flow_get_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_GetNode_ExplicitNodeShowsChainTagsBindingsAndRollup(t *testing.T) {
	sess := authedInspectServer(t, true)

	res, out := callText(t, sess, "flow_get_node", map[string]any{"node": "jukebox"})
	if res.IsError {
		t.Fatalf("get_node errored: %s", out)
	}
	for _, want := range []string{"Jukebox", "jukebox", "repo", "der Plattenspieler",
		"Alpha / Rebuild / Jukebox", "go", "12h 30m", "notebook-a", "/work/jukebox"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q in:\n%s", want, out)
		}
	}
	// The other node's binding must not leak into this node's detail.
	if strings.Contains(out, "/work/alpha") || strings.Contains(out, "notebook-b") {
		t.Errorf("detail leaked another node's binding (ListBindings is owner-wide and must be filtered):\n%s", out)
	}
}

func TestLoopback_GetNode_OmittedUsesTheBoundNode(t *testing.T) {
	sess := authedInspectServer(t, true)
	res, out := callText(t, sess, "flow_get_node", map[string]any{})
	if res.IsError {
		t.Fatalf("get_node without node errored: %s", out)
	}
	if !strings.Contains(out, "Jukebox") {
		t.Fatalf("result = %q, want the directory-bound node", out)
	}
}

func TestLoopback_GetNode_OmittedAndUnboundPointsAtTheBindingTools(t *testing.T) {
	sess := authedInspectServer(t, false)
	res, out := callText(t, sess, "flow_get_node", map[string]any{})
	if !res.IsError {
		t.Fatalf("unbound get_node: want IsError, got %q", out)
	}
	if !strings.Contains(out, "flow_node_binding") {
		t.Fatalf("error = %q, want it to point at flow_node_binding", out)
	}
}

func TestLoopback_GetNode_UnknownNodeErrors(t *testing.T) {
	sess := authedInspectServer(t, true)
	res, out := callText(t, sess, "flow_get_node", map[string]any{"node": "bogus"})
	if !res.IsError || !strings.Contains(out, "unknown project") {
		t.Fatalf("unknown node = (IsError=%v, %q), want IsError + 'unknown project'", res.IsError, out)
	}
}
