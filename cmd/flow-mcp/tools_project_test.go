package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// nodeUpdateRecorder captures what flow_update_node actually sent to the
// backend (the PATCH body and/or the rate POST), so tests can assert partial
// -update semantics — an unset field must stay nil, never leak a zero value.
type nodeUpdateRecorder struct {
	mu               sync.Mutex
	lastUpdate       *apiclient.UpdateNodeFields
	updateCalls      int
	rateCalls        int
	lastRateAmount   *int64
	lastRateCurrency string
}

func (r *nodeUpdateRecorder) recordUpdate(f apiclient.UpdateNodeFields) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdate = &f
	r.updateCalls++
}

func (r *nodeUpdateRecorder) recordRate(amount *int64, currency string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRateAmount = amount
	r.lastRateCurrency = currency
	r.rateCalls++
}

// fakeNodeUpdateBackend serves the nodes list plus the PATCH/rate endpoints
// flow_update_node touches. "n1" (slug "r") is the owned node; "other1"
// (slug "other") resolves client-side but 404s on PATCH/rate, modeling a
// foreign node the server-side ownership guard rejects.
func fakeNodeUpdateBackend(t *testing.T, rec *nodeUpdateRecorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "n1", Name: "Repo", Slug: "r"},
			{ID: "other1", Name: "Other", Slug: "other"},
		})
	})
	mux.HandleFunc("PATCH /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "n1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var f apiclient.UpdateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		rec.recordUpdate(f)
		_ = json.NewEncoder(w).Encode(domain.Node{ID: id, Name: "Repo", Slug: "r"})
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/rate", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "n1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in struct {
			Amount   *int64 `json:"amount"`
			Currency string `json:"currency"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		rec.recordRate(in.Amount, in.Currency)
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux)
}

// authedNodeUpdateServer builds a loopback MCP session bound to node "n1"
// (slug "r"), plus the recorder its backend fills in.
func authedNodeUpdateServer(t *testing.T) (*mcp.ClientSession, *nodeUpdateRecorder) {
	t.Helper()
	rec := &nodeUpdateRecorder{}
	be := fakeNodeUpdateBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "n1", Name: "Repo", Slug: "r"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	return connect(t, h.srv), rec
}

func TestLoopback_UpdateNode_Advertised(t *testing.T) {
	sess, _ := authedNodeUpdateServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_update_node") {
		t.Fatalf("flow_update_node not advertised; got %v", toolNames(tools.Tools))
	}
}

// TestLoopback_UpdateNode_PartialAndAddressing is the TDD anchor from the
// task brief: a single-field update (description) must reach the backend
// with only that field set — the rest of UpdateNodeFields must stay nil, not
// leak the zero value.
func TestLoopback_UpdateNode_PartialAndAddressing(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "description": "neu",
	})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", out)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.lastUpdate == nil || rec.lastUpdate.Description == nil || *rec.lastUpdate.Description != "neu" {
		t.Fatalf("expected description update, got %+v", rec.lastUpdate)
	}
	if rec.lastUpdate.Name != nil {
		t.Errorf("unset field leaked: %+v", rec.lastUpdate)
	}
	if rec.updateCalls != 1 {
		t.Errorf("updateCalls = %d, want 1", rec.updateCalls)
	}
	if rec.rateCalls != 0 {
		t.Errorf("rateCalls = %d, want 0 (no rate fields passed)", rec.rateCalls)
	}
}

// TestLoopback_UpdateNode_DefaultsToBoundNode omits `node` entirely — it must
// resolve via the current directory's binding (h.artifactNode with "").
func TestLoopback_UpdateNode_DefaultsToBoundNode(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{"name": "New Name"})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.lastUpdate == nil || rec.lastUpdate.Name == nil || *rec.lastUpdate.Name != "New Name" {
		t.Fatalf("expected name update via bound-node default, got %+v", rec.lastUpdate)
	}
}

func TestLoopback_UpdateNode_InvalidStatus(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "status": "bogus",
	})
	if !res.IsError || !strings.Contains(out, "status") {
		t.Fatalf("invalid status = (IsError=%v, %q), want a status error", res.IsError, out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (validation must short-circuit before the HTTP call)", rec.updateCalls)
	}
}

func TestLoopback_UpdateNode_ValidStatus(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "status": "paused",
	})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.lastUpdate == nil || rec.lastUpdate.Status == nil || *rec.lastUpdate.Status != "paused" {
		t.Fatalf("expected status update, got %+v", rec.lastUpdate)
	}
}

func TestLoopback_UpdateNode_RateAndClearRateMutuallyExclusive(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	rate := float64(8000)
	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "rate": rate, "clearRate": true,
	})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("rate+clearRate = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.updateCalls != 0 || rec.rateCalls != 0 {
		t.Errorf("HTTP calls made despite validation error: updateCalls=%d rateCalls=%d", rec.updateCalls, rec.rateCalls)
	}
}

func TestLoopback_UpdateNode_SetRateDefaultsCurrencyToEUR(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	rate := float64(8000)
	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "rate": rate,
	})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.lastRateAmount == nil || *rec.lastRateAmount != 8000 {
		t.Fatalf("lastRateAmount = %v, want 8000", rec.lastRateAmount)
	}
	if rec.lastRateCurrency != "EUR" {
		t.Fatalf("lastRateCurrency = %q, want EUR default", rec.lastRateCurrency)
	}
	if rec.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (only rate fields passed)", rec.updateCalls)
	}
}

func TestLoopback_UpdateNode_ClearRate(t *testing.T) {
	sess, rec := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "r", "clearRate": true,
	})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", out)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.rateCalls != 1 {
		t.Fatalf("rateCalls = %d, want 1", rec.rateCalls)
	}
	if rec.lastRateAmount != nil {
		t.Fatalf("lastRateAmount = %v, want nil (cleared)", rec.lastRateAmount)
	}
}

// TestLoopback_UpdateNode_OwnerScope404 mirrors the artifact tools' guard: a
// node ref that resolves client-side (it's in the node list) but whose
// PATCH/rate endpoint the backend 404s — as it would for a foreign/unowned
// node — must come back as an MCP error, never a silent success.
func TestLoopback_UpdateNode_OwnerScope404(t *testing.T) {
	sess, _ := authedNodeUpdateServer(t)

	res, out := callText(t, sess, "flow_update_node", map[string]any{
		"node": "other", "description": "neu",
	})
	if !res.IsError {
		t.Fatalf("update on foreign node = (IsError=%v, %q), want an error", res.IsError, out)
	}
}
