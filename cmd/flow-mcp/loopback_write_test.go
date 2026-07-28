package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func authedWriteServerWithResources(t *testing.T) (*mcp.ClientSession, *handlers) {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	if err := h.registerResources(context.Background(), client); err != nil {
		t.Fatalf("registerResources: %v", err)
	}
	_ = mgr
	return connect(t, h.srv), h
}

func TestLoopback_Resources_BootAndLiveSync(t *testing.T) {
	sess, _ := authedWriteServerWithResources(t)
	ctx := context.Background()

	// boot: the one seeded project doc (d-human) is a resource
	rs, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasResource(rs.Resources, "flow://doc/d-human") {
		t.Fatalf("boot resources = %v, want d-human", resourceURIs(rs.Resources))
	}
	// read returns the (fresh) body
	rr, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "flow://doc/d-human"})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "human note") {
		t.Fatalf("read d-human = (%+v,%v), want the body", rr, err)
	}
	// create in-project → a new resource appears
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/r", "title": "R", "body": "rbody"})
	rs, err = sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after create resources = %v, want new1", resourceURIs(rs.Resources))
	}
	// delete (agent-owned, no confirm needed) → resource removed
	_, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "new1"})
	rs, err = sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("after delete resources still has new1: %v", resourceURIs(rs.Resources))
	}
}

func TestLoopback_Resources_ReconcileExternalCreateDeleteAndRebind(t *testing.T) {
	sess, h := authedWriteServerWithResources(t)
	ctx := context.Background()
	c, err := h.mgr.client(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p1 := "p1"
	external, err := c.CreateDocument(ctx, apiclient.CreateDocumentInput{
		Type: string(domain.DocMemory), NodeID: &p1, Path: "external", Title: "External", Body: "outside MCP",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, out := callText(t, sess, "flow_refresh_resources", map[string]any{})
	if res.IsError || !strings.Contains(out, "Reconciled") {
		t.Fatalf("refresh after external create = (IsError=%v, %q)", res.IsError, out)
	}
	rs, err := sess.ListResources(ctx, nil)
	if err != nil || !hasResource(rs.Resources, docURI(external.ID)) {
		t.Fatalf("after external create reconcile resources=%v err=%v", resourceURIs(rs.Resources), err)
	}

	if err := c.DeleteDocument(ctx, external.ID); err != nil {
		t.Fatal(err)
	}
	res, out = callText(t, sess, "flow_refresh_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("refresh after external delete: %s", out)
	}
	rs, err = sess.ListResources(ctx, nil)
	if err != nil || hasResource(rs.Resources, docURI(external.ID)) {
		t.Fatalf("after external delete reconcile resources=%v err=%v", resourceURIs(rs.Resources), err)
	}

	h.projMu.Lock()
	h.proj = domain.Node{ID: "p2", Name: "Beta", Slug: "beta"}
	h.matched = true
	h.projMu.Unlock()
	res, out = callText(t, sess, "flow_refresh_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("refresh after rebind: %s", out)
	}
	rs, err = sess.ListResources(ctx, nil)
	if err != nil || len(rs.Resources) != 0 {
		t.Fatalf("after rebind resources=%v err=%v, want empty p2 set", resourceURIs(rs.Resources), err)
	}
}

func TestLoopback_Resources_PeriodicReconcileSeesExternalCreate(t *testing.T) {
	sess, h := authedWriteServerWithResources(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := h.mgr.client(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p1 := "p1"
	external, err := c.CreateDocument(ctx, apiclient.CreateDocumentInput{
		Type: string(domain.DocMemory), NodeID: &p1, Path: "periodic", Title: "Periodic", Body: "outside MCP",
	})
	if err != nil {
		t.Fatal(err)
	}
	go h.runResourceReconciler(ctx, 5*time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rs, listErr := sess.ListResources(ctx, nil)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if hasResource(rs.Resources, docURI(external.ID)) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("periodic reconciliation did not expose external create")
}

func hasResource(rs []*mcp.Resource, uri string) bool {
	for _, r := range rs {
		if r.URI == uri {
			return true
		}
	}
	return false
}

func resourceURIs(rs []*mcp.Resource) []string {
	var u []string
	for _, r := range rs {
		u = append(u, r.URI)
	}
	return u
}

// fakeWriteBackend serves the CRUD endpoints the write tools touch, backed by an
// in-memory map. p1 = the resolved project (Alpha).
func fakeWriteBackend(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	p1 := "p1"
	baseTime := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	docs := map[string]domain.Document{
		"d-human":    {ID: "d-human", OwnerID: "u1", NodeID: &p1, Type: domain.DocFree, Path: "notes/keep", Title: "Keep", Body: "## Checklist\n\n- [ ] F40 context\n\nhuman note", UpdatedAt: baseTime},
		"d-memory":   {ID: "d-memory", OwnerID: "u1", NodeID: &p1, Type: domain.DocMemory, Path: "memory/one", Title: "Memory One", Body: "memory body", Priority: 2, ContextMode: domain.ContextModeAuto, UpdatedAt: baseTime},
		"d-memory-2": {ID: "d-memory-2", OwnerID: "u1", NodeID: &p1, Type: domain.DocMemory, Path: "memory/two", Title: "Memory Two", Body: "second memory body", Priority: 1, ContextMode: domain.ContextModeAuto, UpdatedAt: baseTime},
	}
	seq := 0
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("POST /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		seq++
		id := "new" + string(rune('0'+seq))
		d := domain.Document{ID: id, OwnerID: "u1", NodeID: in.NodeID, Type: domain.DocumentType(in.Type), Path: in.Path, Title: in.Title, Body: in.Body, Tags: in.Tags, UpdatedAt: baseTime}
		docs[id] = d
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("POST /api/v1/documents/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		id := r.PathValue("id")
		d, ok := docs[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in apiclient.MoveDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		d.Type = domain.DocumentType(in.Type)
		d.NodeID = in.NodeID
		d.Path = in.Path
		d.Date = in.Date
		docs[id] = d
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("PUT /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.UpdateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.Title, d.Body = in.Title, in.Body
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("PATCH /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.PatchDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if in.ExpectedUpdatedAt != nil && !d.UpdatedAt.Equal(*in.ExpectedUpdatedAt) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "document_conflict", "message": "document changed since it was read",
				"httpStatus": http.StatusConflict, "retryable": true,
				"conflictVersion": d.UpdatedAt.UTC().Format(time.RFC3339Nano),
			})
			return
		}
		if in.Title != nil {
			d.Title = *in.Title
		}
		if in.Body != nil {
			d.Body = *in.Body
		}
		if in.Tags != nil {
			d.Tags = append([]string(nil), (*in.Tags)...)
		}
		d.UpdatedAt = d.UpdatedAt.Add(time.Microsecond)
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("DELETE /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		delete(docs, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/documents/{id}/pin", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Pinned bool `json:"pinned"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.Pinned = in.Pinned
		d.UpdatedAt = d.UpdatedAt.Add(time.Microsecond)
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/v1/documents/{id}/context-mode", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Mode domain.ContextMode `json:"mode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.ContextMode = in.Mode
		d.UpdatedAt = d.UpdatedAt.Add(time.Microsecond)
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/v1/documents/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Archived bool `json:"archived"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		d, ok := docs[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d.Archived = in.Archived
		d.UpdatedAt = d.UpdatedAt.Add(time.Microsecond)
		if in.Archived {
			now := baseTime.Add(time.Hour)
			d.ArchivedAt = &now
			d.Pinned = false
		} else {
			d.ArchivedAt = nil
		}
		docs[d.ID] = d
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/v1/documents/archived", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var out []domain.Document
		for _, d := range docs {
			if d.Archived {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/context", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		proj := domain.Node{ID: p1, Name: "Alpha", Slug: "alpha"}
		cc := usecase.ComposedContext{
			Resolution: usecase.ContextResolution{Repo: &proj, Chain: []domain.Node{proj}},
			Memories:   map[string][]usecase.ContextItem{p1: {}},
			Budget:     usecase.ContextBudget{Cap: 100},
		}
		var ranked []domain.Document
		for _, d := range docs {
			if d.Archived || !contextDocumentType(d.Type) || d.NodeID == nil || *d.NodeID != p1 {
				continue
			}
			item := usecase.ContextItem{ID: d.ID, NodeID: d.NodeID, ScopeLabel: "repo:alpha", Type: d.Type, Path: d.Path, Title: d.Title, Tags: d.Tags, Pinned: d.Pinned, Priority: d.Priority, ContextMode: d.ContextMode, EstTokens: 10}
			cc.Candidates = append(cc.Candidates, item)
			if d.ContextMode.OrAuto() == domain.ContextModeNie {
				cc.Hidden = append(cc.Hidden, item)
				continue
			}
			ranked = append(ranked, d)
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].Priority > ranked[j].Priority })
		for i, d := range ranked {
			item := usecase.ContextItem{ID: d.ID, NodeID: d.NodeID, ScopeLabel: "repo:alpha", Type: d.Type, Path: d.Path, Title: d.Title, Body: d.Body, Pinned: d.Pinned, Priority: d.Priority, ContextMode: d.ContextMode, EstTokens: 10}
			cc.Ranked = append(cc.Ranked, usecase.RankedItem{Item: item, Group: p1, Included: true, Rank: i + 1})
			cc.Memories[p1] = append(cc.Memories[p1], item)
			cc.Budget.Used += item.EstTokens
		}
		_ = json.NewEncoder(w).Encode(cc)
	})
	mux.HandleFunc("POST /api/v1/context/reorder", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		for i, id := range in.IDs {
			d, ok := docs[id]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			d.Priority = len(in.IDs) - i
			docs[id] = d
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "n": len(in.IDs)})
	})
	// list (used by resolveScope's nothing here, but harmless to provide)
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var out []domain.Document
		pid, has := r.URL.Query().Get("projectId"), r.URL.Query().Has("projectId")
		for _, d := range docs {
			if !d.Archived && (!has || (d.NodeID != nil && *d.NodeID == pid)) {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedWriteServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeWriteBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	return connect(t, h.srv)
}

func TestLoopback_WriteTools_Advertised(t *testing.T) {
	sess := authedWriteServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flow_create_doc", "flow_update_doc", "flow_patch_doc", "flow_move_doc", "flow_delete_doc"} {
		if !hasTool(tools.Tools, name) {
			t.Fatalf("%s not advertised; got %v", name, toolNames(tools.Tools))
		}
	}
}

func TestLoopback_MoveGuardAndReclassify(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_move_doc", map[string]any{
		"id": "d-human", "type": "project", "project": "alpha", "path": "readme",
	})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("unguarded move = (IsError=%v, %q), want confirm refusal", res.IsError, out)
	}
	res, out = callText(t, sess, "flow_move_doc", map[string]any{
		"id": "d-human", "type": "project", "project": "alpha", "path": "readme", "confirm": true,
	})
	if res.IsError || !strings.Contains(out, "project") || !strings.Contains(out, "readme") {
		t.Fatalf("confirmed move = (IsError=%v, %q)", res.IsError, out)
	}
}

func TestLoopback_CreateThenGet(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/new", "title": "New", "body": "fresh body",
	})
	if res.IsError {
		t.Fatalf("create errored: %s", out)
	}
	if !strings.Contains(out, "new1") || !strings.Contains(out, "memory") {
		t.Fatalf("create result = %q, want it to name the new id + type", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "new1"})
	if !strings.Contains(got, "fresh body") {
		t.Fatalf("get after create = %q, want the body", got)
	}
}

func TestLoopback_CreateInvalidType(t *testing.T) {
	sess := authedWriteServer(t)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{"type": "bogus", "path": "p", "title": "T", "body": "B"})
	if !res.IsError || !strings.Contains(out, "memory") {
		t.Fatalf("create bad type = (IsError=%v, %q), want IsError listing valid types", res.IsError, out)
	}
}

func TestLoopback_UpdateGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned (free) without confirm → refused
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Hacked"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded update = (IsError=%v, %q), want refusal naming confirm", res.IsError, out)
	}
	// with confirm → allowed, body carried over (partial merge)
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "title": "Edited", "confirm": true})
	if res.IsError {
		t.Fatalf("confirmed update errored: %s", out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "Edited") || !strings.Contains(got, "human note") {
		t.Fatalf("after confirmed partial update = %q, want new title + carried-over body", got)
	}
}

func TestLoopback_UpdateTagsOnlyAndPatchCheckbox(t *testing.T) {
	sess := authedWriteServer(t)

	tags := []string{"reliable"}
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "d-human", "tags": tags, "confirm": true})
	if res.IsError || !strings.Contains(out, `"project":"alpha"`) || !strings.Contains(out, `"version":`) || !strings.Contains(out, `"hash":"sha256:`) {
		t.Fatalf("tags-only update = (IsError=%v, %q), want structured write result", res.IsError, out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "human note") || !strings.Contains(got, "reliable") {
		t.Fatalf("tags-only update clobbered content or tags: %q", got)
	}

	res, out = callText(t, sess, "flow_patch_doc", map[string]any{
		"id": "d-human", "operation": "set_checkbox", "checkbox": "F40 context", "checked": true,
		"label": "F40 — Behoben: CAS-safe", "confirm": true,
	})
	if res.IsError || !strings.Contains(out, `"action":"patched"`) {
		t.Fatalf("checkbox patch = (IsError=%v, %q)", res.IsError, out)
	}
	_, got = callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "- [x] F40 — Behoben: CAS-safe") || strings.Contains(got, "F40 context") {
		t.Fatalf("checkbox was not patched: %q", got)
	}
}

func TestLoopback_UpdateConflictReturnsStructuredError(t *testing.T) {
	sess := authedWriteServer(t)
	baseVersion := "2026-07-15T12:00:00Z"

	res, out := callText(t, sess, "flow_update_doc", map[string]any{
		"id": "d-human", "title": "first", "expectedUpdatedAt": baseVersion, "confirm": true,
	})
	if res.IsError {
		t.Fatalf("first update errored: %s", out)
	}
	res, out = callText(t, sess, "flow_update_doc", map[string]any{
		"id": "d-human", "title": "stale", "expectedUpdatedAt": baseVersion, "confirm": true,
	})
	if !res.IsError || !strings.Contains(out, `"code":"document_conflict"`) || !strings.Contains(out, `"httpStatus":409`) || !strings.Contains(out, `"retryable":true`) || !strings.Contains(out, `"conflictVersion":"2026-07-15T12:00:00.000001Z"`) {
		t.Fatalf("stale update = (IsError=%v, %q), want structured conflict", res.IsError, out)
	}
}

func TestLoopback_PatchCheckboxLabelConflictLeavesWinningBody(t *testing.T) {
	sess := authedWriteServer(t)
	baseVersion := "2026-07-15T12:00:00Z"

	res, out := callText(t, sess, "flow_patch_doc", map[string]any{
		"id": "d-human", "operation": "set_checkbox", "checkbox": "F40 context", "checked": true,
		"label": "F40 — Behoben", "expectedUpdatedAt": baseVersion, "confirm": true,
	})
	if res.IsError {
		t.Fatalf("winning checkbox patch errored: %s", out)
	}

	res, out = callText(t, sess, "flow_patch_doc", map[string]any{
		"id": "d-human", "operation": "set_checkbox", "checkbox": "F40 — Behoben", "checked": false,
		"label": "F40 — Wieder offen", "expectedUpdatedAt": baseVersion, "confirm": true,
	})
	if !res.IsError || !strings.Contains(out, `"code":"document_conflict"`) {
		t.Fatalf("stale checkbox patch = (IsError=%v, %q), want structured conflict", res.IsError, out)
	}
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !strings.Contains(got, "- [x] F40 — Behoben") || strings.Contains(got, "Wieder offen") {
		t.Fatalf("stale patch changed the winning body: %q", got)
	}
}

func TestLoopback_UpdateAgentOwnedNoConfirm(t *testing.T) {
	sess := authedWriteServer(t)
	_, _ = callText(t, sess, "flow_create_doc", map[string]any{"type": "memory", "path": "notes/a", "title": "A", "body": "B"})
	res, out := callText(t, sess, "flow_update_doc", map[string]any{"id": "new1", "body": "B2"})
	if res.IsError {
		t.Fatalf("agent-owned update without confirm should succeed, got error: %s", out)
	}
}

func TestLoopback_DeleteGuard(t *testing.T) {
	sess := authedWriteServer(t)
	// human-owned delete without confirm → refused
	res, out := callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human"})
	if !res.IsError || !strings.Contains(out, "confirm") {
		t.Fatalf("guarded delete = (IsError=%v, %q), want refusal", res.IsError, out)
	}
	// with confirm → gone
	res, _ = callText(t, sess, "flow_delete_doc", map[string]any{"id": "d-human", "confirm": true})
	if res.IsError {
		t.Fatal("confirmed delete should succeed")
	}
	res, _ = callText(t, sess, "flow_get_doc", map[string]any{"id": "d-human"})
	if !res.IsError {
		t.Fatal("get after delete should error (not found)")
	}
}

func TestLoopback_ContextCurationInventoryModeAndOrder(t *testing.T) {
	sess := authedWriteServer(t)

	res, inventory := callText(t, sess, "flow_context_inventory", map[string]any{"cap": 100})
	if res.IsError || !strings.Contains(inventory, `"id":"d-memory"`) || !strings.Contains(inventory, `"state":"included"`) {
		t.Fatalf("inventory = (IsError=%v, %q)", res.IsError, inventory)
	}
	if strings.Contains(inventory, "memory body") || strings.Contains(inventory, `"body"`) {
		t.Fatalf("inventory leaked a body: %q", inventory)
	}
	res, pinned := callText(t, sess, "flow_curate_context", map[string]any{"id": "d-memory", "pinned": true, "cap": 100})
	if res.IsError || !strings.Contains(pinned, `"pinned":true`) {
		t.Fatalf("curate pin = (IsError=%v, %q)", res.IsError, pinned)
	}

	res, invalid := callText(t, sess, "flow_reorder_context", map[string]any{"ids": []string{"d-memory"}, "cap": 100})
	if !res.IsError || !strings.Contains(invalid, "all 2 ranked") {
		t.Fatalf("incomplete reorder = (IsError=%v, %q)", res.IsError, invalid)
	}
	res, reordered := callText(t, sess, "flow_reorder_context", map[string]any{"ids": []string{"d-memory-2", "d-memory"}, "cap": 100})
	if res.IsError || !strings.Contains(reordered, `"count":2`) {
		t.Fatalf("complete reorder = (IsError=%v, %q)", res.IsError, reordered)
	}

	res, curated := callText(t, sess, "flow_curate_context", map[string]any{"id": "d-memory", "mode": "nie", "cap": 100})
	if res.IsError || !strings.Contains(curated, `"state":"hidden"`) || !strings.Contains(curated, `"contextMode":"nie"`) {
		t.Fatalf("curate mode = (IsError=%v, %q)", res.IsError, curated)
	}
	res, archived := callText(t, sess, "flow_curate_context", map[string]any{"id": "d-memory-2", "archived": true, "cap": 100})
	if res.IsError || !strings.Contains(archived, `"state":"archived"`) {
		t.Fatalf("curate archive = (IsError=%v, %q)", res.IsError, archived)
	}
	res, inventory = callText(t, sess, "flow_context_inventory", map[string]any{"cap": 100})
	if res.IsError || strings.Contains(inventory, `"id":"d-memory-2"`) {
		t.Fatalf("archived document remained in inventory = (IsError=%v, %q)", res.IsError, inventory)
	}
}

func TestLoopback_ArchiveGuardListAndRestore(t *testing.T) {
	sess := authedWriteServer(t)

	res, refused := callText(t, sess, "flow_archive_doc", map[string]any{"id": "d-human"})
	if !res.IsError || !strings.Contains(refused, "confirm") {
		t.Fatalf("unguarded archive = (IsError=%v, %q)", res.IsError, refused)
	}
	res, archived := callText(t, sess, "flow_archive_doc", map[string]any{"id": "d-human", "confirm": true})
	if res.IsError || !strings.Contains(archived, `"archived":true`) {
		t.Fatalf("confirmed archive = (IsError=%v, %q)", res.IsError, archived)
	}
	res, listed := callText(t, sess, "flow_list_archived_docs", map[string]any{})
	if res.IsError || !strings.Contains(listed, `"id":"d-human"`) {
		t.Fatalf("archived list = (IsError=%v, %q)", res.IsError, listed)
	}
	if strings.Contains(listed, "human note") || strings.Contains(listed, `"body"`) {
		t.Fatalf("archived list leaked a body: %q", listed)
	}
	res, restored := callText(t, sess, "flow_archive_doc", map[string]any{"id": "d-human", "archived": false, "confirm": true})
	if res.IsError || !strings.Contains(restored, `"archived":false`) {
		t.Fatalf("restore = (IsError=%v, %q)", res.IsError, restored)
	}
}

func TestLoopback_CreateNoneScope(t *testing.T) {
	sess := authedWriteServer(t)
	// Create a doc with project="none" — must produce an unassigned document.
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/unassigned", "title": "Unassigned", "body": "no project", "project": "none",
	})
	if res.IsError {
		t.Fatalf("create with project=none errored: %s", out)
	}
	// Extract the id from the create result ("Created memory [new1] Unassigned · …")
	if !strings.Contains(out, "new1") {
		t.Fatalf("expected id new1 in create result, got %q", out)
	}
	// Round-trip via flow_get_doc: formatDoc renders "project: —" when NodeID is nil.
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": "new1"})
	if !strings.Contains(got, "project: —") {
		t.Fatalf("get after create-with-none = %q; want 'project: —' (nil NodeID)", got)
	}
}

func TestLoopback_Resources_OutOfScopeCreateNotRegistered(t *testing.T) {
	sess, _ := authedWriteServerWithResources(t)
	ctx := context.Background()

	// baseline: only the seeded in-scope doc (d-human) is a resource
	rs, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeCount := len(rs.Resources)

	// create explicitly unassigned → NodeID nil → not in scope of p1
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/oob", "title": "OutOfBand", "body": "nowhere", "project": "none",
	})
	if res.IsError {
		t.Fatalf("out-of-scope create errored: %s", out)
	}

	// resource list must not grow — the new doc is not in p1
	rs, err = sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Resources) != beforeCount {
		t.Fatalf("resource count after out-of-scope create: got %d, want %d; resources: %v",
			len(rs.Resources), beforeCount, resourceURIs(rs.Resources))
	}
	if hasResource(rs.Resources, "flow://doc/new1") {
		t.Fatalf("out-of-scope doc new1 must not appear as a resource; resources: %v", resourceURIs(rs.Resources))
	}
}

func TestLoopback_WriteTools_DegradedRequireLogin(t *testing.T) {
	sess := degradedSession(t)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_create_doc", map[string]any{"type": "memory", "path": "p", "title": "T", "body": "B"}},
		{"flow_update_doc", map[string]any{"id": "d1", "title": "X"}},
		{"flow_delete_doc", map[string]any{"id": "d1"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Fatalf("%s degraded = (IsError=%v, %q), want Login required", tc.name, res.IsError, got)
		}
	}
}

// TestLoopback_Reauth_TransparentRetryOn401 proves mgr.Do resets+rebuilds+retries
// within a single tool call when the backend returns a 401 on the first request.
func TestLoopback_Reauth_TransparentRetryOn401(t *testing.T) {
	var mu sync.Mutex
	first := true
	p1 := "p1"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]domain.Document{{ID: "d1", OwnerID: "u1", NodeID: &p1, Type: domain.DocMemory, Path: "p", Title: "t"}})
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	builds := 0
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) {
		builds++
		return apiclient.New(be.URL, "tok"), nil
	}, nil)
	srv, h := newServerH(mgr)
	mgr.onAuth = nil // seed resolution directly; fixture lacks V0 endpoints
	h.projMu.Lock()
	h.proj, h.matched = domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}, true
	h.projMu.Unlock()
	sess := connect(t, srv)

	res, out := callText(t, sess, "flow_list_docs", map[string]any{})
	if res.IsError {
		t.Fatalf("list after transparent reauth = error %q, want success", out)
	}
	if !strings.Contains(out, "d1") {
		t.Fatalf("list = %q, want d1 after retry", out)
	}
	if builds < 2 {
		t.Fatalf("builds = %d, want >= 2 (initial + rebuild on 401)", builds)
	}
}

func TestLoopback_PatchAndUpdateReportBodyDelta(t *testing.T) {
	sess := authedWriteServer(t)

	// Ein kleiner Patch: die Delta-Felder sind da, der Guard (Task 5) greift nicht.
	res, out := callText(t, sess, "flow_patch_doc", map[string]any{
		"id": "d-human", "operation": "set_checkbox", "checkbox": "F40 context", "checked": true,
		"confirm": true,
	})
	if res.IsError {
		t.Fatalf("checkbox patch errored: %s", out)
	}
	for _, want := range []string{`"bytesBefore":`, `"bytesAfter":`, `"linesBefore":`, `"linesAfter":`} {
		if !strings.Contains(out, want) {
			t.Fatalf("patch response %q is missing %s", out, want)
		}
	}

	// flow_update_doc mit leerem Body: bytesAfter/linesAfter MÜSSEN als 0
	// erscheinen. Mit `int` + omitempty würde json genau diese Null
	// verschlucken — also ausgerechnet den Totalverlust.
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-memory", "body": ""})
	if res.IsError {
		t.Fatalf("emptying update errored: %s", out)
	}
	if !strings.Contains(out, `"bytesAfter":0`) || !strings.Contains(out, `"linesAfter":0`) {
		t.Fatalf("emptying update response = %q, want bytesAfter:0 and linesAfter:0", out)
	}

	// Ein Update ohne body lässt den Text unangetastet — kein Delta.
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-memory-2", "title": "Renamed"})
	if res.IsError {
		t.Fatalf("title-only update errored: %s", out)
	}
	if strings.Contains(out, `"bytesBefore":`) || strings.Contains(out, `"linesAfter":`) {
		t.Fatalf("title-only update response = %q, want no delta fields", out)
	}

	// create hat kein Vorher.
	res, out = callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/delta", "title": "D", "body": "fresh",
	})
	if res.IsError {
		t.Fatalf("create errored: %s", out)
	}
	if strings.Contains(out, `"bytesBefore":`) {
		t.Fatalf("create response = %q, want no delta fields", out)
	}
}
