package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newReadmeCockpitServer builds the minimal *Server nodeCockpitData needs
// (mirrors webui_cockpit_test.go's newCockpitTestServer, but package-internal
// so it can call the unexported nodeCockpitData directly — Task 7 tests the
// README build BEFORE its templ consumer exists, Task 8). Every field
// nodeCockpitData touches on an unwired/zero-value path is guarded (`if
// s.X.Y != nil`), so bindings/logo/context/activity are deliberately left
// zero here; only what's on the always-run path is wired.
func newReadmeCockpitServer(t *testing.T) (*Server, *testutil.FakeNodeStore, *testutil.FakeDocumentStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	ds := testutil.NewFakeDocumentStore()
	arts := testutil.NewFakeArtifactStore()
	srv := &Server{
		Clock:             clk,
		GetNode:           usecase.GetNode{Nodes: ps},
		NodeAncestors:     usecase.NodeAncestors{Nodes: ps},
		Stats:             usecase.StatsComputer{Sessions: ss, Nodes: ps, Clock: clk, Loc: time.Local},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		GetRunningSession: usecase.GetRunningSession{Sessions: ss},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		ListDocuments:     usecase.ListDocuments{Docs: ds},
		ListArtifacts:     usecase.ListArtifacts{Nodes: ps, Artifacts: arts},
	}
	return srv, ps, ds
}

func readmeTestUser() domain.User {
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	return u
}

// TestNodeCockpitData_RendersReadme pins FR-A's backend contract: a node
// carrying an own `readme.md`/`README` doc gets it rendered into d.Readme
// (sanitized HTML, wikilinks/artifacts resolved via the same pipeline as
// buildDocumentVM) with d.HasReadme = true.
func TestNodeCockpitData_RendersReadme(t *testing.T) {
	s, ps, ds := newReadmeCockpitServer(t)
	if _, err := ps.Create(t.Context(), domain.Node{ID: "n1", OwnerID: "u1", Kind: domain.KindRepo, Name: "R", Slug: "r"}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nid := "n1"
	if _, err := ds.Create(t.Context(), domain.Document{
		ID: "d1", OwnerID: "u1", NodeID: &nid, Path: "README.md",
		Title: "R", Body: "# Hallo\n\nWelt.",
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	req := httptest.NewRequest("GET", "/nodes/n1", nil)
	d, err := s.nodeCockpitData(req, readmeTestUser(), "n1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if !d.HasReadme {
		t.Fatalf("HasReadme = false, want true")
	}
	if !strings.Contains(string(d.Readme), "Hallo") {
		t.Errorf("rendered README missing content: %q", d.Readme)
	}
}

// TestNodeCockpitData_NoReadmeEmptyState pins the empty-state contract: a
// node without a readme doc gets HasReadme=false and a populated
// ReadmeNewHref (the empty-state "create it" link), never a crash.
func TestNodeCockpitData_NoReadmeEmptyState(t *testing.T) {
	s, ps, _ := newReadmeCockpitServer(t)
	if _, err := ps.Create(t.Context(), domain.Node{ID: "n1", OwnerID: "u1", Kind: domain.KindRepo, Name: "R", Slug: "r"}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	req := httptest.NewRequest("GET", "/nodes/n1", nil)
	d, err := s.nodeCockpitData(req, readmeTestUser(), "n1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if d.HasReadme {
		t.Errorf("HasReadme = true without a readme doc")
	}
	if d.ReadmeNewHref == "" {
		t.Errorf("empty-state ReadmeNewHref not set")
	}
}

// TestNodeCockpitData_ReadmeUppercaseMDSuffix pins the case-insensitivity fix:
// findReadme lowercases the path BEFORE trimming ".md", so an uppercase
// extension ("README.MD") still matches. Regression guard for a bug where
// lowercasing happened AFTER TrimSuffix(".md"), so "README.MD" was left as
// "readme.md" (never trimmed) and failed to match "readme".
func TestNodeCockpitData_ReadmeUppercaseMDSuffix(t *testing.T) {
	s, ps, ds := newReadmeCockpitServer(t)
	if _, err := ps.Create(t.Context(), domain.Node{ID: "n1", OwnerID: "u1", Kind: domain.KindRepo, Name: "R", Slug: "r"}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nid := "n1"
	if _, err := ds.Create(t.Context(), domain.Document{
		ID: "d1", OwnerID: "u1", NodeID: &nid, Path: "README.MD",
		Title: "R", Body: "# Upper\n\nCase.",
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	req := httptest.NewRequest("GET", "/nodes/n1", nil)
	d, err := s.nodeCockpitData(req, readmeTestUser(), "n1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if !d.HasReadme {
		t.Fatalf("HasReadme = false, want true for README.MD (uppercase suffix)")
	}
	if !strings.Contains(string(d.Readme), "Upper") {
		t.Errorf("rendered README missing content: %q", d.Readme)
	}
}

// TestNodeCockpitData_ReadmeOwnNodeOnly pins the no-inheritance rule: a
// readme doc on the PARENT must not surface on the child's cockpit.
func TestNodeCockpitData_ReadmeOwnNodeOnly(t *testing.T) {
	s, ps, ds := newReadmeCockpitServer(t)
	if _, err := ps.Create(t.Context(), domain.Node{ID: "p1", OwnerID: "u1", Kind: domain.KindVorhaben, Name: "P", Slug: "p"}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	pid := "p1"
	if _, err := ps.Create(t.Context(), domain.Node{ID: "c1", OwnerID: "u1", Kind: domain.KindRepo, Name: "C", Slug: "c", ParentID: &pid}); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if _, err := ds.Create(t.Context(), domain.Document{
		ID: "d1", OwnerID: "u1", NodeID: &pid, Path: "readme",
		Title: "P", Body: "parent readme body",
	}); err != nil {
		t.Fatalf("seed parent readme: %v", err)
	}

	req := httptest.NewRequest("GET", "/nodes/c1", nil)
	d, err := s.nodeCockpitData(req, readmeTestUser(), "c1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if d.HasReadme {
		t.Errorf("child must NOT inherit the parent's readme, got HasReadme=true Readme=%q", d.Readme)
	}
}
