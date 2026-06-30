package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestMoveNode_PostsParent(t *testing.T) {
	t.Parallel()
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/n1/move" {
			b, _ := json.Marshal(map[string]any{})
			var raw map[string]any
			_ = json.NewDecoder(r.Body).Decode(&raw)
			gotBody, _ = raw["parentId"].(string)
			_ = b
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", ParentID: func() *string { s := "p2"; return &s }()})
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	p := "p2"
	n, err := c.MoveNode(context.Background(), "n1", &p)
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != "p2" || n.ParentID == nil || *n.ParentID != "p2" {
		t.Fatalf("MoveNode body=%q result=%+v", gotBody, n)
	}
}

func TestAncestors_Decodes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/nodes/repo1/ancestors" {
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "repo1"}, {ID: "eng1"}})
			return
		}
		t.Errorf("unexpected %s", r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	chain, err := c.Ancestors(context.Background(), "repo1")
	if err != nil || len(chain) != 2 || chain[1].ID != "eng1" {
		t.Fatalf("Ancestors = %+v err=%v", chain, err)
	}
}

func TestResolveEngagement_404IsNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	_, ok, err := c.ResolveEngagement(context.Background(), "github.com/x/y", "m1", "/tmp")
	if err != nil || ok {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestCreateNode_PostsFields(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes" {
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Kind: domain.KindRepo})
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	p := "eng1"
	n, err := c.CreateNode(context.Background(), apiclient.CreateNodeFields{Name: "flow", Kind: "repo", ParentID: &p})
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "repo" || got["parentId"] != "eng1" || n.Kind != domain.KindRepo {
		t.Fatalf("CreateNode body=%v result=%+v", got, n)
	}
	_ = strings.TrimSpace
}

func TestNodeStats_DecodesCorrectly(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/n1/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"totalMin":600,"weekMin":300,"monthMin":1200}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	stats, err := c.NodeStats(context.Background(), "n1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalMin != 600 {
		t.Errorf("TotalMin: got %d, want 600", stats.TotalMin)
	}
	if stats.WeekMin != 300 {
		t.Errorf("WeekMin: got %d, want 300", stats.WeekMin)
	}
	if stats.MonthMin != 1200 {
		t.Errorf("MonthMin: got %d, want 1200", stats.MonthMin)
	}
}
