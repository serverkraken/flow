package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// treeFixture is an engagement with a vorhaben and a repo under it, plus a
// second root — enough to assert indentation, ordering and kind per line.
func treeFixture() []domain.Node {
	eng, vor := "e1", "v1"
	return []domain.Node{
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &vor, Status: domain.NodeActive},
		{ID: "e2", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodePaused},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &eng, Status: domain.NodeActive},
		{ID: "e1", Name: "Zeta", Slug: "zeta", Kind: domain.KindEngagement, Status: domain.NodeActive},
	}
}

func TestFormatNodeTree_IndentsAndNamesKindPerLine(t *testing.T) {
	out := formatNodeTree(treeFixture())
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("tree has %d lines, want 4:\n%s", len(lines), out)
	}
	// Roots alphabetical: Alpha before Zeta.
	if !strings.Contains(lines[0], "Alpha") || !strings.Contains(lines[1], "Zeta") {
		t.Fatalf("roots not alphabetical:\n%s", out)
	}
	// Indentation: the vorhaben under Zeta is deeper than Zeta, the repo deeper still.
	if strings.HasPrefix(lines[2], "  ") == false || strings.HasPrefix(lines[3], "    ") == false {
		t.Fatalf("children are not indented two spaces per level:\n%s", out)
	}
	if strings.HasPrefix(lines[0], " ") {
		t.Fatalf("a root must not be indented:\n%s", out)
	}
	// Every line carries kind, slug, status and id — what flow_create_node needs.
	for _, want := range []string{"engagement", "vorhaben", "repo", "jukebox", "rebuild", "paused", "active", "e1", "v1", "r1"} {
		if !strings.Contains(out, want) {
			t.Errorf("tree missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatNodeTree_UpstreamIsShownWhenSet(t *testing.T) {
	out := formatNodeTree([]domain.Node{
		{ID: "r1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo, Status: domain.NodeActive,
			UpstreamGit: "git@github.com:serverkraken/flow.git"},
	})
	if !strings.Contains(out, "github.com") {
		t.Fatalf("tree must show upstream when set, got %q", out)
	}
}

func TestFormatNodeTree_EmptyPointsAtCreateNode(t *testing.T) {
	out := formatNodeTree(nil)
	if out == "" {
		t.Fatal("formatNodeTree(nil) must return a non-empty message")
	}
	if !strings.Contains(out, "flow_create_node") {
		t.Fatalf("empty message = %q, want it to name flow_create_node (create_name is gone)", out)
	}
	if strings.Contains(out, "create_name") {
		t.Fatalf("empty message = %q, must not mention the removed create_name parameter", out)
	}
}

func TestBuildNodeForest_DanglingParentBecomesARootInsteadOfVanishing(t *testing.T) {
	absent := "not-in-this-list"
	roots := buildNodeForest([]domain.Node{
		{ID: "x1", Name: "Orphan", Slug: "orphan", Kind: domain.KindRepo, ParentID: &absent},
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement},
	})
	if len(roots) != 2 {
		t.Fatalf("forest has %d roots, want 2 (a dangling parent must not hide the node)", len(roots))
	}
	// True roots come before dangling ones.
	if roots[0].Node.ID != "e1" || roots[1].Node.ID != "x1" {
		t.Fatalf("root order = %s,%s; want the true root first", roots[0].Node.ID, roots[1].Node.ID)
	}
}

func TestBuildNodeForest_SiblingsAreNameSortedCaseInsensitively(t *testing.T) {
	parent := "e1"
	roots := buildNodeForest([]domain.Node{
		{ID: "e1", Name: "Root", Slug: "root", Kind: domain.KindEngagement},
		{ID: "b", Name: "beta", Slug: "beta", Kind: domain.KindVorhaben, ParentID: &parent},
		{ID: "a", Name: "Alpha", Slug: "alpha", Kind: domain.KindVorhaben, ParentID: &parent},
	})
	if len(roots) != 1 || len(roots[0].Children) != 2 {
		t.Fatalf("unexpected forest shape: %+v", roots)
	}
	if roots[0].Children[0].Node.ID != "a" {
		t.Fatalf("siblings not case-insensitively name-sorted: %s before %s",
			roots[0].Children[0].Node.Name, roots[0].Children[1].Node.Name)
	}
}

// TestFormatNodeTree_LongNamesAndSlugsPassThroughVerbatim pins the "long" state
// for this surface. A model, not a 375px viewport, reads this output, and a
// truncated slug or id is an UNUSABLE address — it would be passed straight back
// into flow_get_node and fail. So single values are never shortened; only
// repeated enumerations are capped (see joinCapped in the delete report).
func TestFormatNodeTree_LongNamesAndSlugsPassThroughVerbatim(t *testing.T) {
	longName := strings.Repeat("Sehr-Langer-Engagement-Name-", 12)
	longSlug := strings.Repeat("langer-slug-", 12) + "ende"
	out := formatNodeTree([]domain.Node{
		{ID: "e1", Name: longName, Slug: longSlug, Kind: domain.KindEngagement, Status: domain.NodeActive},
	})
	if !strings.Contains(out, longName) {
		t.Errorf("a long name must not be truncated:\n%s", out)
	}
	if !strings.Contains(out, longSlug) {
		t.Errorf("a long slug must not be truncated — it is the node's address:\n%s", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("no ellipsis may appear in a single value:\n%s", out)
	}
	if n := strings.Count(out, "\n"); n != 0 {
		t.Errorf("one node must stay one line, got %d newlines:\n%s", n, out)
	}
}

func TestNodeKindGlyph_UsesMonospaceGlyphsOnly(t *testing.T) {
	for _, k := range []domain.NodeKind{domain.KindEngagement, domain.KindVorhaben, domain.KindRepo, domain.KindBranch} {
		g := nodeKindGlyph(k)
		if g == "" {
			t.Fatalf("nodeKindGlyph(%s) is empty", k)
		}
		if len([]rune(g)) != 1 {
			t.Fatalf("nodeKindGlyph(%s) = %q, want exactly one glyph rune (AGENTS.md bans emoji pictograms)", k, g)
		}
	}
	if nodeKindGlyph(domain.NodeKind("bogus")) == "" {
		t.Fatal("an unknown kind must still get a fallback glyph")
	}
}

func TestFormatDeleteImpact_DeletableNodeReportsMinutesAndOwnArtifactsOnly(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node:         domain.Node{ID: "l1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, LogoRef: "sha256:abc"},
		OwnArtifacts: 3,
		HasLogo:      true,
		Rollup:       apiclient.NodeRollup{TotalMin: 750},
	})
	for _, want := range []string{`Would delete repo "Jukebox" (jukebox)`, "12h 30m", "3 artifact", "1 logo",
		"No children, no project documents", "confirm=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "session") {
		t.Errorf("report must speak of minutes, not sessions (NodeStats has no session count):\n%s", out)
	}
}

func TestFormatDeleteImpact_NoWorktimeAndNoLogoReadCleanly(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node: domain.Node{ID: "l1", Name: "Leer", Slug: "leer", Kind: domain.KindRepo},
	})
	if !strings.Contains(out, "No booked worktime") {
		t.Errorf("report must state the empty worktime case:\n%s", out)
	}
	if !strings.Contains(out, "no logo") {
		t.Errorf("report must state the no-logo case instead of '0 logo':\n%s", out)
	}
	if strings.Contains(out, "0h 00m of booked") {
		t.Errorf("report must not print a zero duration:\n%s", out)
	}
}

func TestFormatDeleteImpact_BlockedByChildrenAndProjectDocs(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node:        domain.Node{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben},
		Children:    []domain.Node{{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}},
		ProjectDocs: []domain.Document{{ID: "d1", Path: "projekt/rebuild", Type: domain.DocProject}},
	})
	for _, want := range []string{"Cannot delete", "rebuild", "jukebox", "flow_move_node",
		"projekt/rebuild", "flow_move_doc"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "confirm=true") {
		t.Errorf("a blocked report must not invite confirm=true:\n%s", out)
	}
}

// TestFormatDeleteImpact_ManyChildrenAreCappedNotDumped is the "long" state for a
// text surface: the count stays exact, the enumeration stays bounded.
func TestFormatDeleteImpact_ManyChildrenAreCappedNotDumped(t *testing.T) {
	children := make([]domain.Node, 42)
	for i := range children {
		children[i] = domain.Node{ID: fmt.Sprintf("c%d", i), Slug: fmt.Sprintf("child-%02d", i), Kind: domain.KindRepo}
	}
	out := formatDeleteImpact(deleteImpact{
		Node:     domain.Node{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben},
		Children: children,
	})
	if !strings.Contains(out, "42 child node(s)") {
		t.Errorf("the exact count must survive the cap:\n%s", out)
	}
	if !strings.Contains(out, "and 32 more") {
		t.Errorf("the enumeration must be capped at %d with a remainder:\n%s", maxDeleteImpactItems, out)
	}
	if strings.Contains(out, "child-41") {
		t.Errorf("the enumeration must not dump every child:\n%s", out)
	}
	if strings.Count(out, "\n") > 3 {
		t.Errorf("a blocked report must stay a few lines, got:\n%s", out)
	}
}

func TestJoinCapped(t *testing.T) {
	if got := joinCapped([]string{"a", "b"}, 10); got != "a, b" {
		t.Errorf("joinCapped under the cap = %q, want a plain join", got)
	}
	if got := joinCapped([]string{"a", "b", "c"}, 2); got != "a, b … and 1 more" {
		t.Errorf("joinCapped over the cap = %q", got)
	}
	if got := joinCapped(nil, 3); got != "" {
		t.Errorf("joinCapped(nil) = %q, want empty", got)
	}
}

func TestDeleteImpactBlocked(t *testing.T) {
	if (deleteImpact{}).blocked() {
		t.Error("an empty impact must not be blocked")
	}
	if !(deleteImpact{Children: []domain.Node{{ID: "x"}}}).blocked() {
		t.Error("children must block")
	}
	if !(deleteImpact{ProjectDocs: []domain.Document{{ID: "d"}}}).blocked() {
		t.Error("project documents must block")
	}
}
