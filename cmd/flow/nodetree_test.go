package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func p(s string) *string { return &s }

func TestBuildTree_ForestAndOrder(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "eng1", Name: "Privat", Kind: domain.KindEngagement},
		{ID: "repoB", Name: "beta", Kind: domain.KindRepo, ParentID: p("eng1")},
		{ID: "repoA", Name: "alpha", Kind: domain.KindRepo, ParentID: p("eng1")},
		{ID: "eng0", Name: "AAA", Kind: domain.KindEngagement},
		{ID: "orphan", Name: "ghost", Kind: domain.KindRepo, ParentID: p("missing")}, // dangling → treated as root
	}
	roots := buildTree(nodes)
	// Roots are name-sorted: AAA, Privat, then the dangling orphan (parent absent).
	if len(roots) != 3 {
		t.Fatalf("roots = %d, want 3", len(roots))
	}
	if roots[0].Node.Name != "AAA" || roots[1].Node.Name != "Privat" {
		t.Fatalf("root order = %q,%q", roots[0].Node.Name, roots[1].Node.Name)
	}
	// Children name-sorted under Privat: alpha, beta.
	priv := roots[1]
	if len(priv.Children) != 2 || priv.Children[0].Node.Name != "alpha" || priv.Children[1].Node.Name != "beta" {
		t.Fatalf("Privat children = %+v", priv.Children)
	}
}

func TestRenderTree_Indents(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement},
		{ID: "repo1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: p("eng1")},
	}
	var sb strings.Builder
	renderTree(buildTree(nodes), &sb)
	out := sb.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d:\n%s", len(lines), out)
	}
	if strings.HasPrefix(lines[0], " ") {
		t.Errorf("root line should not be indented: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") || !strings.Contains(lines[1], "flow") {
		t.Errorf("child line should be indented and name the repo: %q", lines[1])
	}
}
