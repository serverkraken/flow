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
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeContextBackend serves GET /api/v1/context and PUT /api/v1/context/active.
func fakeContextBackend(t *testing.T, capturedNode *string, capturedBody *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/context", func(w http.ResponseWriter, r *http.Request) {
		if capturedNode != nil {
			*capturedNode = r.URL.Query().Get("node")
		}
		slug := "alpha"
		cc := usecase.ComposedContext{
			Resolution:   usecase.ContextResolution{Repo: &domain.Node{Slug: slug}},
			Instructions: []usecase.ContextItem{{ID: "i1", Body: "do the thing"}},
			Budget:       usecase.ContextBudget{Cap: 4000, Used: 3},
		}
		_ = json.NewEncoder(w).Encode(cc)
	})
	mux.HandleFunc("PUT /api/v1/context/active", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Node string `json:"node"`
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if capturedNode != nil {
			*capturedNode = body.Node
		}
		if capturedBody != nil {
			*capturedBody = body.Body
		}
		_ = json.NewEncoder(w).Encode(apiclient.SetActiveContextResult{ID: "ac1", UpdatedAt: "2026-01-01T00:00:00Z"})
	})
	return httptest.NewServer(mux)
}

func authedContextServer(t *testing.T, capturedNode *string, capturedBody *string) (*mcp.ClientSession, *handlers) {
	t.Helper()
	be := fakeContextBackend(t, capturedNode, capturedBody)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	h.listProjects = func(context.Context) ([]domain.Node, error) {
		return []domain.Node{
			proj,
			{ID: "p2", Name: "Beta", Slug: "beta"},
			{
				ID:          "p-flow",
				Name:        "Flow",
				Slug:        "github-com-serverkraken-flow",
				Kind:        domain.KindRepo,
				OriginSlug:  "github.com/serverkraken/flow",
				UpstreamGit: "git@github.com:serverkraken/flow.git",
			},
		}, nil
	}
	return connect(t, h.srv), h
}

func TestLoopback_GetContext_Advertised(t *testing.T) {
	sess, _ := authedContextServer(t, nil, nil)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if !hasTool(tools.Tools, "flow_get_context") {
		t.Fatalf("flow_get_context not advertised; got %v", toolNames(tools.Tools))
	}
	if !hasTool(tools.Tools, "flow_set_active_context") {
		t.Fatalf("flow_set_active_context not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_GetContext_ReturnsJSON(t *testing.T) {
	var capturedNode string
	sess, _ := authedContextServer(t, &capturedNode, nil)

	res, got := callText(t, sess, "flow_get_context", map[string]any{})
	if res.IsError {
		t.Fatalf("flow_get_context IsError: %s", got)
	}
	// Result must be JSON containing the resolved node and at least one instruction.
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "do the thing") {
		t.Fatalf("flow_get_context = %q, want JSON with 'alpha' and 'do the thing'", got)
	}
	// Default resolution uses the resolved project slug (option B).
	if capturedNode != "alpha" {
		t.Fatalf("GET /api/v1/context got node=%q, want 'alpha' (resolved project slug)", capturedNode)
	}
}

func TestLoopback_GetContext_ForwardsHardCapAndProfile(t *testing.T) {
	var gotCap, gotProfile string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/context", func(w http.ResponseWriter, r *http.Request) {
		gotCap = r.URL.Query().Get("cap")
		gotProfile = r.URL.Query().Get("profile")
		_ = json.NewEncoder(w).Encode(usecase.ComposedContext{Budget: usecase.ContextBudget{Cap: 2200, Used: 2200}})
	})
	be := httptest.NewServer(mux)
	defer be.Close()
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"})
	sess := connect(t, h.srv)

	res, out := callText(t, sess, "flow_get_context", map[string]any{"cap": 2200, "profile": "handoff"})
	if res.IsError {
		t.Fatalf("flow_get_context errored: %s", out)
	}
	if gotCap != "2200" || gotProfile != "handoff" {
		t.Fatalf("context query cap=%q profile=%q, want 2200/handoff", gotCap, gotProfile)
	}
}

func TestLoopback_GetContext_RepoOverride(t *testing.T) {
	var capturedNode string
	sess, _ := authedContextServer(t, &capturedNode, nil)

	res, got := callText(t, sess, "flow_get_context", map[string]any{"repo": "beta"})
	if res.IsError {
		t.Fatalf("flow_get_context (repo override) IsError: %s", got)
	}
	if capturedNode != "beta" {
		t.Fatalf("GET /api/v1/context got node=%q, want 'beta' (repo override)", capturedNode)
	}
}

func TestLoopback_GetContext_RepoOverrideByUpstreamGit(t *testing.T) {
	var capturedNode string
	sess, _ := authedContextServer(t, &capturedNode, nil)

	res, got := callText(t, sess, "flow_get_context", map[string]any{"repo": "github.com/serverkraken/flow"})
	if res.IsError {
		t.Fatalf("flow_get_context (upstream override) IsError: %s", got)
	}
	if capturedNode != "github-com-serverkraken-flow" {
		t.Fatalf("GET /api/v1/context got node=%q, want canonical node slug", capturedNode)
	}
}

func TestLoopback_SetActiveContext_RepoOverrideByUpstreamGit(t *testing.T) {
	var capturedNode string
	sess, _ := authedContextServer(t, &capturedNode, nil)

	res, got := callText(t, sess, "flow_set_active_context", map[string]any{
		"repo": "git@github.com:serverkraken/flow.git",
		"body": "next",
	})
	if res.IsError {
		t.Fatalf("flow_set_active_context (upstream override) IsError: %s", got)
	}
	if capturedNode != "github-com-serverkraken-flow" {
		t.Fatalf("PUT /api/v1/context/active got node=%q, want canonical node slug", capturedNode)
	}
}

func TestLoopback_SetActiveContext_UpdatesAndReturnsConfirmation(t *testing.T) {
	var capturedBody string
	sess, _ := authedContextServer(t, nil, &capturedBody)

	res, got := callText(t, sess, "flow_set_active_context", map[string]any{
		"body": "I was in task C4; next: commit + slice-C gate",
		"tags": []any{"b3", "mcp"},
	})
	if res.IsError {
		t.Fatalf("flow_set_active_context IsError: %s", got)
	}
	if !strings.Contains(got, `"action":"active_context_updated"`) || !strings.Contains(got, `"project":"alpha"`) || !strings.Contains(got, `"id":"ac1"`) || !strings.Contains(got, `"hash":"sha256:`) {
		t.Fatalf("flow_set_active_context = %q, want structured write metadata", got)
	}
	if capturedBody != "I was in task C4; next: commit + slice-C gate" {
		t.Fatalf("PUT body = %q, want the input body forwarded verbatim", capturedBody)
	}
}

func TestLoopback_SetActiveContext_FailsClosedWhenUnresolved(t *testing.T) {
	var capturedBody string
	sess, h := authedContextServer(t, nil, &capturedBody)
	h.projMu.Lock()
	h.proj = domain.Node{}
	h.matched = false
	h.projMu.Unlock()

	res, got := callText(t, sess, "flow_set_active_context", map[string]any{"body": "must not be written"})
	if !res.IsError || !strings.Contains(got, "flow_bind_project") {
		t.Fatalf("unresolved set_active_context = (IsError=%v, %q), want fail-closed guidance", res.IsError, got)
	}
	if capturedBody != "" {
		t.Fatalf("unresolved set_active_context reached backend with body %q", capturedBody)
	}
}

func TestLoopback_SetActiveContext_DegradedRequiresLogin(t *testing.T) {
	sess := degradedSession(t)
	res, got := callText(t, sess, "flow_set_active_context", map[string]any{"body": "x"})
	if !res.IsError || !strings.Contains(got, "Login required") {
		t.Fatalf("degraded set_active_context = (IsError=%v, %q), want IsError + 'Login required'", res.IsError, got)
	}
}
