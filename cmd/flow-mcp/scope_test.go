package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func fakeProjects() []domain.Node {
	return []domain.Node{
		{ID: "p1", Name: "Alpha", Slug: "alpha"},
		{ID: "p2", Name: "Beta", Slug: "beta"},
	}
}

func TestResolveScope_DefaultUsesMatchedProject(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Node{ID: "p1", Name: "Alpha"}}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID == nil || *sc.nodeID != "p1" {
		t.Fatalf("nodeID = %v, want &\"p1\"", sc.nodeID)
	}
	if !strings.Contains(sc.label, "Alpha") {
		t.Fatalf("label = %q, want it to mention Alpha", sc.label)
	}
}

func TestResolveScope_DefaultUnmatchedIsGlobal(t *testing.T) {
	h := &handlers{matched: false}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID != nil {
		t.Fatalf("nodeID = %v, want nil (global)", sc.nodeID)
	}
}

func TestResolveWriteScope_FailsClosedWithoutResolvedProject(t *testing.T) {
	h := &handlers{matched: false}
	if _, err := h.resolveWriteScope(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "flow_bind_project") {
		t.Fatalf("unresolved default write err = %v, want fail-closed guidance", err)
	}
	if _, err := h.resolveWriteScope(context.Background(), "global"); err == nil || !strings.Contains(err.Error(), "none") {
		t.Fatalf("global write err = %v, want explicit none guidance", err)
	}
	sc, err := h.resolveWriteScope(context.Background(), "none")
	if err != nil || sc.nodeID == nil || *sc.nodeID != "none" {
		t.Fatalf("explicit none write scope = (%+v,%v)", sc, err)
	}
}

func TestResolveScope_GlobalAndNoneSentinels(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Node{ID: "p1"}}
	g, err := h.resolveScope(context.Background(), "global")
	if err != nil {
		t.Fatal(err)
	}
	if g.nodeID != nil {
		t.Fatalf("global nodeID = %v, want nil", g.nodeID)
	}
	n, err := h.resolveScope(context.Background(), "none")
	if err != nil {
		t.Fatal(err)
	}
	if n.nodeID == nil || *n.nodeID != "none" {
		t.Fatalf("none nodeID = %v, want &\"none\"", n.nodeID)
	}
}

func TestResolveScope_ExplicitBySlugAndName(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		return fakeProjects(), nil
	}}
	bySlug, err := h.resolveScope(context.Background(), "beta")
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.nodeID == nil || *bySlug.nodeID != "p2" {
		t.Fatalf("by slug = %v, want &\"p2\"", bySlug.nodeID)
	}
	byName, err := h.resolveScope(context.Background(), "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if byName.nodeID == nil || *byName.nodeID != "p1" {
		t.Fatalf("by name = %v, want &\"p1\"", byName.nodeID)
	}
	if calls != 1 {
		t.Fatalf("listProjects called %d times, want 1 (cached after first fetch)", calls)
	}
}

func TestResolveScope_ExplicitByUpstreamGit(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		return []domain.Node{{
			ID:          "p-flow",
			Name:        "Flow",
			Slug:        "github-com-serverkraken-flow",
			Kind:        domain.KindRepo,
			OriginSlug:  "github.com/serverkraken/flow",
			UpstreamGit: "git@github.com:serverkraken/flow.git",
		}}, nil
	}}

	for _, ref := range []string{
		"github.com/serverkraken/flow",
		"https://github.com/serverkraken/flow.git",
		"git@github.com:serverkraken/flow.git",
	} {
		sc, err := h.resolveScope(context.Background(), ref)
		if err != nil {
			t.Fatalf("resolve %q: %v", ref, err)
		}
		if sc.nodeID == nil || *sc.nodeID != "p-flow" {
			t.Fatalf("resolve %q = %v, want p-flow", ref, sc.nodeID)
		}
	}
}

func TestMatchNode_AmbiguousUpstreamGitDoesNotChooseArbitrarily(t *testing.T) {
	ps := []domain.Node{
		{ID: "one", Slug: "one", Kind: domain.KindRepo, UpstreamGit: "git@github.com:o/r.git"},
		{ID: "two", Slug: "two", Kind: domain.KindRepo, UpstreamGit: "https://github.com/o/r.git"},
	}
	if got, ok := matchNode(ps, "github.com/o/r"); ok {
		t.Fatalf("ambiguous remote resolved to %q", got.ID)
	}
}

func TestMatchNode_AmbiguousBareSlugDoesNotChooseArbitrarily(t *testing.T) {
	workID, privateID := "work", "private"
	ps := []domain.Node{
		{ID: "one", Slug: "api", ParentID: &workID},
		{ID: "two", Slug: "api", ParentID: &privateID},
	}
	if got, ok := matchNode(ps, "api"); ok {
		t.Fatalf("ambiguous slug resolved to %q", got.ID)
	}
}

func TestLookupNode_AmbiguousBareSlugReturnsGuidanceWithoutRefresh(t *testing.T) {
	workID, privateID := "work", "private"
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		return []domain.Node{
			{ID: workID, Slug: "work"},
			{ID: privateID, Slug: "private"},
			{ID: "one", Slug: "api", ParentID: &workID},
			{ID: "two", Slug: "api", ParentID: &privateID},
		}, nil
	}}
	_, err := h.lookupNode(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "work/api") || !strings.Contains(err.Error(), "private/api") {
		t.Fatalf("lookup ambiguity error = %v, want both qualified paths", err)
	}
	if calls != 1 {
		t.Fatalf("ambiguous lookup refreshed %d times, want one authoritative list", calls)
	}
}

func TestResolveScope_UnknownRefreshesOnceThenErrors(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		return fakeProjects(), nil // never contains "gamma"
	}}
	_, err := h.resolveScope(context.Background(), "gamma")
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !strings.Contains(err.Error(), "gamma") || !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("error %q should name the bad ref and list known slugs", err)
	}
	if calls != 2 {
		t.Fatalf("listProjects called %d times, want 2 (initial + one refresh on miss)", calls)
	}
}

func TestResolveScope_NewlyCreatedFoundAfterRefresh(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		if calls == 1 {
			return fakeProjects(), nil // gamma not yet visible
		}
		return append(fakeProjects(), domain.Node{ID: "p3", Name: "Gamma", Slug: "gamma"}), nil
	}}
	sc, err := h.resolveScope(context.Background(), "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID == nil || *sc.nodeID != "p3" {
		t.Fatalf("nodeID = %v, want &\"p3\" after refresh", sc.nodeID)
	}
}

func TestResolveScope_ListProjectsError(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		return nil, errors.New("boom")
	}}
	_, err := h.resolveScope(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected the underlying list error to surface")
	}
}

func TestProjectName(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		return fakeProjects(), nil
	}}
	p1 := "p1"
	if got := h.projectName(context.Background(), &p1); got != "Alpha" {
		t.Fatalf("projectName(&p1) = %q, want Alpha", got)
	}
	if got := h.projectName(context.Background(), nil); got != "" {
		t.Fatalf("projectName(nil) = %q, want \"\"", got)
	}
	unknown := "pX"
	if got := h.projectName(context.Background(), &unknown); got != "" {
		t.Fatalf("projectName(unknown) = %q, want \"\"", got)
	}
}

func TestCheckType(t *testing.T) {
	if got, err := checkType(""); err != nil || got != "" {
		t.Fatalf("checkType(\"\") = (%q,%v), want (\"\",nil)", got, err)
	}
	if got, err := checkType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("checkType(\"memory\") = (%q,%v), want (memory,nil)", got, err)
	}
	_, err := checkType("bogus")
	if err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("checkType(\"bogus\") err = %v, want it to list valid types", err)
	}
}

func TestNodeTarget_ExplicitRefWins(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	h.proj, h.matched = domain.Node{ID: "bound1", Name: "Bound", Slug: "bound"}, true

	got, err := h.nodeTarget(context.Background(), fakeProjects()[0].Slug)
	if err != nil {
		t.Fatalf("nodeTarget: %v", err)
	}
	if got.ID != fakeProjects()[0].ID {
		t.Fatalf("nodeTarget(explicit) = %q, want the named node %q", got.ID, fakeProjects()[0].ID)
	}
}

func TestNodeTarget_OmittedUsesBoundNode(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	h.proj, h.matched = domain.Node{ID: "bound1", Name: "Bound", Slug: "bound"}, true

	got, err := h.nodeTarget(context.Background(), "  ")
	if err != nil {
		t.Fatalf("nodeTarget: %v", err)
	}
	if got.ID != "bound1" {
		t.Fatalf("nodeTarget(\"\") = %q, want the directory-bound node", got.ID)
	}
}

func TestNodeTarget_OmittedAndUnboundIsAnActionableError(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	// h.matched stays false: nothing is bound to this directory.
	_, err := h.nodeTarget(context.Background(), "")
	if err == nil {
		t.Fatal("unbound nodeTarget: want an error, got nil")
	}
	for _, want := range []string{"flow_node_binding", "flow_bind_project", "node="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q so the model knows how to fix it", err.Error(), want)
		}
	}
	var g errGuard
	if !errors.As(err, &g) {
		t.Fatalf("error type = %T, want errGuard", err)
	}
}

func TestPrefixGuard(t *testing.T) {
	guard := prefixGuard("parent", errGuard{errors.New(`unknown project "x"`)})
	var g errGuard
	if !errors.As(guard, &g) {
		t.Fatalf("prefixGuard dropped the guard type: %T", guard)
	}
	if !strings.HasPrefix(guard.Error(), "parent: ") {
		t.Fatalf("prefixGuard message = %q, want a 'parent: ' prefix", guard.Error())
	}
	// A transport/auth failure must NOT be downgraded to a validation error.
	transport := errors.New("flow server error listing projects: dial tcp: refused")
	if got := prefixGuard("parent", transport); got != transport {
		t.Fatalf("prefixGuard(transport) = %v, want the original error untouched", got)
	}
}

// TestRefreshResolved_DropsTheNodeCache pins the tenant boundary: the node-ref
// cache must not survive an authenticated client rebuild, because the rebuilt
// client can belong to a different owner. Without this, lookupNode's
// "known slugs: …" message leaks the previous owner's slugs (scope.go:87).
func TestRefreshResolved_DropsTheNodeCache(t *testing.T) {
	var fetches int
	ownerASlugs := []domain.Node{{ID: "a1", Name: "Owner A Node", Slug: "owner-a-secret"}}
	ownerBSlugs := []domain.Node{{ID: "b1", Name: "Owner B Node", Slug: "owner-b-node"}}

	h := &handlers{resources: map[string]string{}}
	h.listProjects = func(context.Context) ([]domain.Node, error) {
		fetches++
		if fetches == 1 {
			return ownerASlugs, nil
		}
		return ownerBSlugs, nil
	}

	// Owner A warms the cache.
	if _, err := h.nodeList(context.Background(), false); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	h.projMu.Lock()
	warmed := h.projFetched
	h.projMu.Unlock()
	if !warmed {
		t.Fatal("cache was not warmed")
	}

	// A client rebuild happens (new identity). refreshResolved must invalidate.
	//
	// Deviation from the brief: passing a nil *apiclient.Client here panics —
	// projectresolve.Resolve (internal/projectresolve/resolve.go:61) always
	// calls c.ResolveNode, which dereferences the client unconditionally
	// (internal/adapter/apiclient/client.go:98); there is no nil-client guard
	// anywhere in that chain. That gap predates this task and lives in
	// internal/, which this task must not touch. A throwaway 404-everything
	// server gives resolveProject the same graceful "no project" outcome
	// (ResolveNode's 404 branch, projectbindings.go:47-49) without the panic.
	be := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(be.Close)
	c := apiclient.New(be.URL, "test-token")
	h.refreshResolved(context.Background(), c)

	h.projMu.Lock()
	stillFetched, cached := h.projFetched, h.projects
	h.projMu.Unlock()
	if stillFetched || cached != nil {
		t.Fatalf("cache survived the rebuild: projFetched=%v projects=%v", stillFetched, cached)
	}

	// The next lookup must therefore see Owner B only.
	//
	// Deviation from the brief: lookupNode's miss message echoes the queried
	// ref verbatim via %q (scope.go:87, unchanged) — so a whole-message
	// substring check for "owner-a-secret" would fail even with the cache
	// correctly invalidated, since that IS the ref being queried. What must
	// not leak is the "known slug: …" list, which reflects the cache; check
	// that part specifically.
	_, err := h.lookupNode(context.Background(), "owner-a-secret")
	if err == nil {
		t.Fatal("Owner A's slug still resolves after the identity change")
	}
	const marker = "known slug: "
	idx := strings.Index(err.Error(), marker)
	if idx < 0 {
		t.Fatalf("miss message %q has no known-slug list to check", err.Error())
	}
	knownSlugs := err.Error()[idx+len(marker):]
	if strings.Contains(knownSlugs, "owner-a-secret") {
		t.Fatalf("the miss message's known-slug list leaks the previous owner's slug: %v", err)
	}
	if !strings.Contains(knownSlugs, "owner-b-node") {
		t.Fatalf("the miss message should list the CURRENT owner's slugs, got: %v", err)
	}
}
