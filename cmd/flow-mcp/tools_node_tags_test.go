package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// tagRecorder captures the PUT body so the replace semantics are provable.
type tagRecorder struct {
	mu     sync.Mutex
	nodeID string
	tags   []string
	calls  int
	sent   bool
}

func (r *tagRecorder) snapshot() (string, []string, int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodeID, r.tags, r.calls, r.sent
}

func fakeNodeTagBackend(t *testing.T, rec *tagRecorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, Status: domain.NodeActive},
		})
	})
	// setNodeTags re-reads the node by id before printing (Finding 2 of the
	// whole-branch review), so every test that reaches that code path needs
	// this endpoint served too.
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.Node{
			ID: r.PathValue("id"), Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, Status: domain.NodeActive,
		})
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tags []string `json:"tags"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.nodeID, rec.tags, rec.calls = r.PathValue("id"), body.Tags, rec.calls+1
		rec.sent = strings.Contains(string(raw), `"tags"`)
		rec.mu.Unlock()
		out := make([]domain.Tag, 0, len(body.Tags))
		for i, tg := range body.Tags {
			out = append(out, domain.Tag{ID: fmt.Sprintf("t%d", i), Slug: tg, Display: tg})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedNodeTagServer(t *testing.T) (*mcp.ClientSession, *tagRecorder) {
	t.Helper()
	rec := &tagRecorder{}
	be := fakeNodeTagBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_SetNodeTags_Advertised(t *testing.T) {
	sess, _ := authedNodeTagServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_set_node_tags") {
		t.Fatalf("flow_set_node_tags not advertised; got %v", toolNames(tools.Tools))
	}
	// The description must warn that this REPLACES the set.
	for _, tool := range tools.Tools {
		if tool.Name == "flow_set_node_tags" && !strings.Contains(strings.ToUpper(tool.Description), "REPLACE") {
			t.Errorf("description must state the replace semantics: %q", tool.Description)
		}
	}
}

func TestLoopback_SetNodeTags_ReplacesAndReportsTheResultingSet(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "jukebox", "tags": []any{"go", "audio"},
	})
	if res.IsError {
		t.Fatalf("set tags errored: %s", out)
	}
	nodeID, tags, calls, _ := rec.snapshot()
	if calls != 1 || nodeID != "r1" {
		t.Fatalf("calls=%d nodeID=%q, want one PUT on r1", calls, nodeID)
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "audio" {
		t.Fatalf("sent tags = %v, want [go audio]", tags)
	}
	for _, want := range []string{"go", "audio", "now has"} {
		if !strings.Contains(out, want) {
			t.Errorf("result missing %q in: %s", want, out)
		}
	}
}

func TestLoopback_SetNodeTags_EmptyListClearsTheSet(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "jukebox", "tags": []any{},
	})
	if res.IsError {
		t.Fatalf("clearing tags errored: %s", out)
	}
	_, tags, calls, sent := rec.snapshot()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 — [] is a real clear, not a no-op", calls)
	}
	if len(tags) != 0 {
		t.Fatalf("sent tags = %v, want an empty list", tags)
	}
	if !sent {
		t.Fatal(`the request body must carry a "tags" key even when clearing`)
	}
	if !strings.Contains(out, "no tags") {
		t.Fatalf("result = %q, want it to state the empty result", out)
	}
}

func TestLoopback_SetNodeTags_OmittedTagsIsAnErrorNotAnAccidentalClear(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"node": "jukebox"})
	if !res.IsError {
		t.Fatalf("omitted tags: want IsError, got %q", out)
	}
	// tags has no `omitempty` (Task 8 brief), so the MCP SDK declares it required
	// in the JSON schema and rejects an omitted key at schema-validation time —
	// the handler's own "tags is required" guard never runs. Either way the
	// property must be named and the handler must never fire.
	if !strings.Contains(out, "required") || !strings.Contains(out, "tags") {
		t.Fatalf("error = %q, want it to name tags as required", out)
	}
	if _, _, calls, _ := rec.snapshot(); calls != 0 {
		t.Fatalf("calls = %d, want 0 — an omitted list must never silently clear", calls)
	}
}

func TestLoopback_SetNodeTags_ExplicitNullIsAnErrorNotAnAccidentalClear(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	// An explicit JSON null is a different wire shape than an omitted key:
	// the key is present, so schema validation lets it through (a Go slice
	// field's generated schema allows type "null"), and it decodes to a nil
	// slice in the handler — indistinguishable there from an omitted field.
	// This path never touches the SDK's schema-validation rejection; it must
	// be caught by the handler's own nil guard instead.
	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"node": "jukebox", "tags": nil})
	if !res.IsError {
		t.Fatalf("explicit null tags: want IsError, got %q", out)
	}
	if !strings.Contains(out, `tags is required`) {
		t.Fatalf("error = %q, want the handler's nil-guard message", out)
	}
	if _, _, calls, _ := rec.snapshot(); calls != 0 {
		t.Fatalf("calls = %d, want 0 — an explicit null must never silently clear", calls)
	}
}

func TestLoopback_SetNodeTags_OmittedNodeUsesTheBoundNode(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"tags": []any{"go"}})
	if res.IsError {
		t.Fatalf("set tags on the bound node errored: %s", out)
	}
	if nodeID, _, _, _ := rec.snapshot(); nodeID != "r1" {
		t.Fatalf("nodeID = %q, want the directory-bound node r1", nodeID)
	}
}

// TestLoopback_SetNodeTags_ReReadsRenamedNodeBeforePrinting is Finding 2 of the
// whole-branch review: nodeTarget's contract (scope.go) guarantees only
// Node.ID is fresh — the omitted-node branch returns the auth-time bound
// snapshot, which goes stale the moment the node is renamed (by
// flow_update_node or a human in the TUI/WebUI) while the agent session runs.
// flow_get_node re-reads by id before printing; flow_set_node_tags must too,
// or it hands the model a slug that the next flow_get_node call rejects as
// "unknown project".
func TestLoopback_SetNodeTags_ReReadsRenamedNodeBeforePrinting(t *testing.T) {
	rec := &tagRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	// The node list still reports the STALE identity — this is the cache
	// nodeTarget's omitted-node branch is seeded from (managerFor below).
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "r1", Name: "Jukbox", Slug: "jukbox", Kind: domain.KindRepo, Status: domain.NodeActive},
		})
	})
	// GetNode reports the CORRECTED identity — a rename that happened after the
	// bound snapshot was taken.
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.Node{
			ID: "r1", Name: "Jukebox", Slug: "jukebox-corrected", Kind: domain.KindRepo, Status: domain.NodeActive,
		})
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tags []string `json:"tags"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.nodeID, rec.tags, rec.calls = r.PathValue("id"), body.Tags, rec.calls+1
		rec.mu.Unlock()
		out := make([]domain.Tag, 0, len(body.Tags))
		for i, tg := range body.Tags {
			out = append(out, domain.Tag{ID: fmt.Sprintf("t%d", i), Slug: tg, Display: tg})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	// The bound snapshot itself carries the STALE name/slug, mirroring what
	// h.resolved() held before the rename.
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukbox", Slug: "jukbox", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"tags": []any{"go"}})
	if res.IsError {
		t.Fatalf("set tags errored: %s", out)
	}
	if nodeID, _, _, _ := rec.snapshot(); nodeID != "r1" {
		t.Fatalf("nodeID = %q, want r1 (the write itself uses the fresh id regardless)", nodeID)
	}
	if strings.Contains(out, "jukbox") && !strings.Contains(out, "jukebox-corrected") {
		t.Errorf("result printed the stale slug %q instead of re-reading the node:\n%s", "jukbox", out)
	}
	if !strings.Contains(out, "jukebox-corrected") {
		t.Errorf("result must print the freshly re-read slug jukebox-corrected:\n%s", out)
	}
	if !strings.Contains(out, "Jukebox") {
		t.Errorf("result must print the freshly re-read name Jukebox:\n%s", out)
	}
}
