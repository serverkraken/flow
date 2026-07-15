package main

import (
	"context"
	"errors"
	"strings"
	"testing"

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
