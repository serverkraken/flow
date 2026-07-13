package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// runNodeList/Create/Move are the testable cores (no clientFromStore/env).

// fakeNodeClient backs an httptest.Server that records PATCH /nodes/{id} and
// POST /nodes/{id}/rate bodies, and resolves slugs from nodeBySlug for
// resolveSlug's GET /api/v1/nodes call. apiclient.Client is a concrete struct
// (not an interface), so "faking" it means standing up a real server.
type fakeNodeClient struct {
	t          *testing.T
	nodeBySlug map[string]string

	lastUpdate       *apiclient.UpdateNodeFields
	rateCalls        int
	lastRateAmount   *int64
	lastRateCurrency string
}

func newFakeNodeClient(t *testing.T) *fakeNodeClient {
	t.Helper()
	return &fakeNodeClient{t: t, nodeBySlug: map[string]string{}}
}

func (fc *fakeNodeClient) client() *apiclient.Client {
	fc.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fc.handle))
	fc.t.Cleanup(srv.Close)
	return apiclient.New(srv.URL, "tkn")
}

func (fc *fakeNodeClient) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
		nodes := make([]domain.Node, 0, len(fc.nodeBySlug))
		for slug, id := range fc.nodeBySlug {
			nodes = append(nodes, domain.Node{ID: id, Slug: slug})
		}
		_ = json.NewEncoder(w).Encode(nodes)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/nodes/"):
		var f apiclient.UpdateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		fc.lastUpdate = &f
		_ = json.NewEncoder(w).Encode(domain.Node{})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rate"):
		fc.rateCalls++
		var body struct {
			Amount   *int64 `json:"amount"`
			Currency string `json:"currency"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fc.lastRateAmount = body.Amount
		fc.lastRateCurrency = body.Currency
		_, _ = w.Write([]byte("null"))
	default:
		fc.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}
}

func TestRunNodeUpdate_SendsOnlySetFields(t *testing.T) {
	fc := newFakeNodeClient(t)
	fc.nodeBySlug["r"] = "n1"
	desc := "neu"
	err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{Description: &desc}, nil)
	if err != nil {
		t.Fatalf("runNodeUpdate: %v", err)
	}
	if fc.lastUpdate == nil || fc.lastUpdate.Description == nil || *fc.lastUpdate.Description != "neu" {
		t.Fatalf("expected description update, got %+v", fc.lastUpdate)
	}
	if fc.lastUpdate.Name != nil || fc.lastUpdate.Icon != nil {
		t.Errorf("unset fields leaked as non-nil: %+v", fc.lastUpdate)
	}
	if fc.rateCalls != 0 {
		t.Errorf("rate touched without --rate/--clear-rate")
	}
}

func TestRunNodeUpdate_RateAndClearRate(t *testing.T) {
	fc := newFakeNodeClient(t)
	fc.nodeBySlug["r"] = "n1"
	amount := int64(8000)
	if err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{}, &rateChange{amount: &amount, currency: "EUR"}); err != nil {
		t.Fatalf("set rate: %v", err)
	}
	if fc.lastRateAmount == nil || *fc.lastRateAmount != 8000 || fc.lastRateCurrency != "EUR" {
		t.Errorf("rate set wrong: amt=%v cur=%q", fc.lastRateAmount, fc.lastRateCurrency)
	}
	if err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{}, &rateChange{clear: true}); err != nil {
		t.Fatalf("clear rate: %v", err)
	}
	if fc.lastRateAmount != nil {
		t.Errorf("clear-rate should send nil amount, got %v", fc.lastRateAmount)
	}
}

func TestNodeUpdateCmd_ClearRateAndRateMutuallyExclusive(t *testing.T) {
	cmd := nodeUpdateCmd()
	cmd.SetArgs([]string{"r", "--clear-rate", "--rate", "1"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want mutually-exclusive error, got %v", err)
	}
}

func TestNodeUpdateCmd_InvalidStatus(t *testing.T) {
	cmd := nodeUpdateCmd()
	cmd.SetArgs([]string{"r", "--status", "bogus"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--status must be") {
		t.Fatalf("want status usage error, got %v", err)
	}
}

func TestRunNodeCreate_PostsKindAndParent(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "eng1", Slug: "privat", Kind: domain.KindEngagement}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Name: "flow", Slug: "flow", Kind: domain.KindRepo})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeCreate(context.Background(), c, &out, "flow", "repo", "privat", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "repo" || got["parentId"] != "eng1" {
		t.Fatalf("posted = %v", got)
	}
	if !strings.Contains(out.String(), "flow") {
		t.Errorf("output missing node name: %q", out.String())
	}
}

func TestRunNodeList_Tree(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, true, "", "all"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 || strings.HasPrefix(lines[0], " ") || !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("tree output:\n%s", out.String())
	}
}

func TestRunNodeList_KindFilter(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, false, "engagement", "all"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "flow") || !strings.Contains(out.String(), "privat") {
		t.Fatalf("kind filter wrong:\n%s", out.String())
	}
}

func TestRunNodeMove_CycleSurfaced(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{
				{ID: "a", Slug: "a"}, {ID: "b", Slug: "b"},
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/move"):
			http.Error(w, "move would create a cycle", http.StatusConflict)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	err := runNodeMove(context.Background(), c, &out, "a", "b")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}
