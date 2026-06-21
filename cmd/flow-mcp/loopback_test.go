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

// fakeBackend serves the minimal flow REST endpoints the spine touches.
func fakeBackend(t *testing.T, docs int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("/api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("projectId") != "p1" {
			http.Error(w, "unexpected projectId", http.StatusBadRequest)
			return
		}
		out := make([]domain.Document, docs)
		for i := range out {
			out[i] = domain.Document{ID: "d", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "t"}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()
	cl := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	sess, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestLoopback_ProjectContext_Authed(t *testing.T) {
	ctx := context.Background()
	be := fakeBackend(t, 2)
	defer be.Close()
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}

	sess := connect(t, newServer(client, true, proj, true))

	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if !hasTool(tools.Tools, "flow_project_context") {
		t.Fatalf("flow_project_context not advertised; got %v", toolNames(tools.Tools))
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "flow_project_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", text(res))
	}
	got := text(res)
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "2") {
		t.Fatalf("project context = %q, want it to mention Alpha and 2 docs", got)
	}
}

func TestLoopback_ProjectContext_DegradedRequiresLogin(t *testing.T) {
	ctx := context.Background()
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "flow_project_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError || !strings.Contains(text(res), "Login required") {
		t.Fatalf("degraded call = (IsError=%v, %q), want IsError + 'Login required'", res.IsError, text(res))
	}
}

func hasTool(ts []*mcp.Tool, name string) bool {
	for _, t := range ts {
		if t.Name == name {
			return true
		}
	}
	return false
}

func toolNames(ts []*mcp.Tool) []string {
	var n []string
	for _, t := range ts {
		n = append(n, t.Name)
	}
	return n
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
