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

// readFixture is the document set the 2b loopback backend serves.
func readFixture() []domain.Document {
	p1, p2 := "p1", "p2"
	return []domain.Document{
		{ID: "d1", OwnerID: "u1", ProjectID: &p1, Type: domain.DocMemory, Path: "notes/arch", Title: "Arch", Body: "the needle lives here", Tags: []string{"go", "design"}},
		{ID: "d2", OwnerID: "u1", ProjectID: &p1, Type: domain.DocFree, Path: "notes/todo", Title: "Todo", Body: "links [[notes/arch]]", Tags: []string{"go"}},
		{ID: "d3", OwnerID: "u1", ProjectID: &p2, Type: domain.DocMemory, Path: "notes/arch", Title: "Beta Arch", Body: "beta body", Tags: []string{"beta"}},
		{ID: "d4", OwnerID: "u1", Type: domain.DocFree, Path: "global-note", Title: "Global", Body: "no project", Tags: nil},
	}
}

// scopedMatch reports whether doc d is in the projectId filter (nil → all,
// "none" → unassigned, else equality) — mirroring the real backend.
func scopedMatch(d domain.Document, projectId string, hasProjectId bool) bool {
	if !hasProjectId {
		return true
	}
	switch projectId {
	case "none":
		return d.ProjectID == nil
	default:
		return d.ProjectID != nil && *d.ProjectID == projectId
	}
}

// fakeReadBackend serves the read endpoints flow-mcp's read tools touch.
func fakeReadBackend(t *testing.T) *httptest.Server {
	t.Helper()
	docs := readFixture()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Project{
			{ID: "p1", Name: "Alpha", Slug: "alpha"},
			{ID: "p2", Name: "Beta", Slug: "beta"},
		})
	})
	mux.HandleFunc("GET /api/v1/documents/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.CollectTags(docs)) // global owner-wide counts
	})
	mux.HandleFunc("GET /api/v1/documents/{id}/backlinks", func(w http.ResponseWriter, r *http.Request) {
		var out []domain.BacklinkRef
		if r.PathValue("id") == "d1" { // d2 links to d1
			out = []domain.BacklinkRef{{ID: "d2", Path: "notes/todo", Title: "Todo", Type: domain.DocFree}}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, d := range docs {
			if d.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(d)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		pid, hasPid := q.Get("projectId"), q.Has("projectId")
		query := q.Get("q")
		if query != "" { // search branch
			var hits []domain.SearchHit
			for _, d := range docs {
				if !scopedMatch(d, pid, hasPid) {
					continue
				}
				if strings.Contains(strings.ToLower(d.Title+" "+d.Body), strings.ToLower(query)) {
					hits = append(hits, domain.SearchHit{Document: d, Snippet: d.Body})
				}
			}
			_ = json.NewEncoder(w).Encode(hits)
			return
		}
		var out []domain.Document // list branch
		for _, d := range docs {
			if scopedMatch(d, pid, hasPid) {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

// authedReadServer builds an MCP server authed and scoped to project Alpha (p1).
func authedReadServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeReadBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}
	return connect(t, newServer(client, true, proj, true))
}

func callText(t *testing.T, sess *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res, text(res)
}

func TestLoopback_ReadTools_Advertised(t *testing.T) {
	sess := authedReadServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_project_context", "flow_search_docs", "flow_list_docs", "flow_get_doc", "flow_list_tags", "flow_backlinks"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_ListDocs_ScopeAndType(t *testing.T) {
	sess := authedReadServer(t)

	// default scope = Alpha (p1) → d1, d2
	_, def := callText(t, sess, "flow_list_docs", map[string]any{})
	if !strings.Contains(def, "d1") || !strings.Contains(def, "d2") || strings.Contains(def, "d3") {
		t.Fatalf("default list = %q, want d1+d2 only", def)
	}
	// type filter memory → d1 only (d2 is free)
	_, mem := callText(t, sess, "flow_list_docs", map[string]any{"type": "memory"})
	if !strings.Contains(mem, "d1") || strings.Contains(mem, "d2") {
		t.Fatalf("type=memory list = %q, want d1 only", mem)
	}
	// explicit project by slug → Beta (p2) → d3
	_, beta := callText(t, sess, "flow_list_docs", map[string]any{"project": "beta"})
	if !strings.Contains(beta, "d3") || strings.Contains(beta, "d1") {
		t.Fatalf("project=beta list = %q, want d3 only", beta)
	}
	// global → all four
	_, all := callText(t, sess, "flow_list_docs", map[string]any{"project": "global"})
	for _, id := range []string{"d1", "d2", "d3", "d4"} {
		if !strings.Contains(all, id) {
			t.Fatalf("global list = %q, missing %s", all, id)
		}
	}
	// none → unassigned → d4
	_, none := callText(t, sess, "flow_list_docs", map[string]any{"project": "none"})
	if !strings.Contains(none, "d4") || strings.Contains(none, "d1") {
		t.Fatalf("project=none list = %q, want d4 only", none)
	}
}

func TestLoopback_ListDocs_UnknownProjectErrors(t *testing.T) {
	sess := authedReadServer(t)
	res, got := callText(t, sess, "flow_list_docs", map[string]any{"project": "bogus"})
	if !res.IsError || !strings.Contains(got, "unknown project") {
		t.Fatalf("unknown project = (IsError=%v, %q), want IsError + 'unknown project' (never a silent empty)", res.IsError, got)
	}
}

func TestLoopback_Search_Scoped(t *testing.T) {
	sess := authedReadServer(t)
	_, hit := callText(t, sess, "flow_search_docs", map[string]any{"query": "needle"})
	if !strings.Contains(hit, "d1") {
		t.Fatalf("search 'needle' (Alpha) = %q, want d1", hit)
	}
	// invalid type → error listing valid types
	res, bad := callText(t, sess, "flow_search_docs", map[string]any{"query": "needle", "type": "bogus"})
	if !res.IsError || !strings.Contains(bad, "memory") {
		t.Fatalf("type=bogus = (IsError=%v, %q), want IsError listing valid types", res.IsError, bad)
	}
}

func TestLoopback_GetDoc_ByIDAndByPath(t *testing.T) {
	sess := authedReadServer(t)
	_, byID := callText(t, sess, "flow_get_doc", map[string]any{"id": "d1"})
	if !strings.Contains(byID, "the needle lives here") || !strings.Contains(byID, "Alpha") {
		t.Fatalf("get_doc id=d1 = %q, want body + project name", byID)
	}
	// by path in the default (Alpha) scope resolves to d1, NOT the same-path d3 in Beta
	_, byPath := callText(t, sess, "flow_get_doc", map[string]any{"path": "notes/arch"})
	if !strings.Contains(byPath, "id: d1") {
		t.Fatalf("get_doc path=notes/arch (Alpha) = %q, want d1 (scope-disambiguated from d3)", byPath)
	}
}

func TestLoopback_ListTags_And_Backlinks(t *testing.T) {
	sess := authedReadServer(t)
	_, tags := callText(t, sess, "flow_list_tags", map[string]any{"project": "global"})
	if !strings.Contains(tags, "go") {
		t.Fatalf("global tags = %q, want 'go'", tags)
	}
	_, bl := callText(t, sess, "flow_backlinks", map[string]any{"id": "d1"})
	if !strings.Contains(bl, "d2") {
		t.Fatalf("backlinks d1 = %q, want d2", bl)
	}
}

func TestLoopback_ReadTools_DegradedRequireLogin(t *testing.T) {
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_search_docs", map[string]any{"query": "x"}},
		{"flow_list_docs", map[string]any{}},
		{"flow_get_doc", map[string]any{"id": "d1"}},
		{"flow_list_tags", map[string]any{}},
		{"flow_backlinks", map[string]any{"id": "d1"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Fatalf("%s degraded = (IsError=%v, %q), want IsError + 'Login required'", tc.name, res.IsError, got)
		}
	}
}
