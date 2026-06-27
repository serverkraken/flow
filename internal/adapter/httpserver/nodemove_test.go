package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newNodesSrv builds a Server over fake stores and returns the do() helper, the
// fake NodeStore (for direct seeding) and the authenticated owner id.
func newNodesSrv(t *testing.T) (do func(method, path, body string) *http.Response, ns *testutil.FakeNodeStore, ownerID string) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns = testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()

	srv := &httpserver.Server{
		Verifier:   testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:     usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:        sse.NewBus(),
		Clock:      clk,
		ListNodes:  usecase.ListNodes{Nodes: ns},
		GetNode:    usecase.GetNode{Nodes: ns},
		DeleteNode: usecase.DeleteNode{Nodes: ns},
		MoveNode:   usecase.MoveNode{Nodes: ns},
		ResolveEngagement: usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bs, Nodes: ns}, Nodes: ns},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
		BindNode:          usecase.BindNode{Bindings: bs, Nodes: ns, IDs: ids, Clock: clk},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	do = func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := do("GET", "/api/v1/me", "")
	var u domain.User
	_ = json.NewDecoder(res.Body).Decode(&u)
	_ = res.Body.Close()
	return do, ns, u.ID
}

// seedNode inserts a node owned by ownerID straight into the fake store.
func seedNode(t *testing.T, ns *testutil.FakeNodeStore, owner, id string, kind domain.NodeKind, parent *string) {
	t.Helper()
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: id, OwnerID: owner, Kind: kind, ParentID: parent,
		Name: id, Slug: id, Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func ptr(s string) *string { return &s }

func TestMoveNode_OK(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "eng2", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("POST", "/api/v1/nodes/repo1/move", `{"parentId":"eng2"}`)
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("move status %d, want 200", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if n.ParentID == nil || *n.ParentID != "eng2" {
		t.Fatalf("parent = %v, want eng2", n.ParentID)
	}
}

func TestMoveNode_Cycle409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "vorA", domain.KindVorhaben, ptr("eng1"))
	seedNode(t, ns, owner, "vorB", domain.KindVorhaben, ptr("vorA")) // vorB descends from vorA

	// Moving vorA under its own descendant vorB → cycle. (kind vorhaben→vorhaben is allowed.)
	res := do("POST", "/api/v1/nodes/vorA/move", `{"parentId":"vorB"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("cycle status %d, want 409", res.StatusCode)
	}
}

func TestMoveNode_InvalidKind400(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "eng2", domain.KindEngagement, nil)

	// An engagement may never have a parent → ErrInvalidNode.
	res := do("POST", "/api/v1/nodes/eng1/move", `{"parentId":"eng2"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid-kind status %d, want 400", res.StatusCode)
	}
}

func TestMoveNode_NotFound404(t *testing.T) {
	do, _, _ := newNodesSrv(t)
	res := do("POST", "/api/v1/nodes/ghost/move", `{"parentId":null}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("not-found status %d, want 404", res.StatusCode)
	}
}

func TestDeleteNode_WithChildren409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("DELETE", "/api/v1/nodes/eng1", "") // has a child → RESTRICT
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("delete-with-children status %d, want 409", res.StatusCode)
	}
}
