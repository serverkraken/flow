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

func TestFormatNodeDetail_ShowsChainRootToLeafTagsBindingsAndRollup(t *testing.T) {
	out := formatNodeDetail(nodeDetail{
		Node: domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo,
			Status: domain.NodeActive, Description: "der Plattenspieler",
			UpstreamGit: "git@github.com:serverkraken/jukebox.git"},
		// Ancestors returns leaf→root; the breadcrumb must print root→leaf.
		Chain: []domain.Node{
			{ID: "r1", Name: "Jukebox"}, {ID: "v1", Name: "Rebuild"}, {ID: "e1", Name: "Alpha"},
		},
		Tags: []domain.Tag{{Slug: "go", Display: "go"}, {Slug: "audio", Display: "audio"}},
		Bindings: []domain.ProjectBinding{
			{NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/jukebox"},
			{NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
		},
		Rollup: apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300},
	})
	for _, want := range []string{"Jukebox", "jukebox", "repo", "active", "r1",
		"der Plattenspieler", "github.com/serverkraken/jukebox",
		"Alpha / Rebuild / Jukebox", "go", "audio",
		"12h 30m", "2h 00m", "5h 00m",
		"notebook-a", "m1", "/work/jukebox"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Jukebox / Rebuild / Alpha") {
		t.Errorf("breadcrumb is leaf→root; it must be printed root→leaf:\n%s", out)
	}
}

func TestFormatNodeDetail_EmptyTagsAndBindingsAreStatedNotOmitted(t *testing.T) {
	out := formatNodeDetail(nodeDetail{
		Node: domain.Node{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
	})
	if !strings.Contains(out, "bindings: none") {
		t.Errorf("detail must state that there are no bindings:\n%s", out)
	}
	// A model reading this output needs a next step, not just an absence —
	// flow_bind_project is the registered tool that creates a binding (see
	// cmd/flow-mcp/server.go). flow_node_binding is ALSO registered by now
	// (flow_node_binding action="bind" is the general form; flow_bind_project
	// is the narrower, cwd-flavoured one), but formatNodeDetail deliberately
	// names only the narrower tool here — this assertion checks that, not
	// that flow_node_binding is unregistered.
	if !strings.Contains(out, "flow_bind_project") {
		t.Errorf("empty bindings must point at flow_bind_project as the next step:\n%s", out)
	}
	if !strings.Contains(out, "tags: none") {
		t.Errorf("detail must state that there are no tags:\n%s", out)
	}
	// flow_set_node_tags now exists (this task), so the empty-tags message must
	// point at it as the next step — the same pattern formatNodeTree uses for an
	// empty tree (flow_create_node) and the bindings line uses for no bindings
	// (flow_bind_project). flow_update_node does not cover tags, so it must not
	// be named here.
	if !strings.Contains(out, "flow_set_node_tags") {
		t.Errorf("empty tags must point at flow_set_node_tags as the next step:\n%s", out)
	}
	if strings.Contains(out, "flow_update_node") {
		t.Errorf("empty tags must not name flow_update_node, which does not set tags:\n%s", out)
	}
	if strings.Contains(out, "description:") || strings.Contains(out, "upstream:") {
		t.Errorf("unset optional fields must be omitted entirely:\n%s", out)
	}
}

// TestFormatNodeDetail_RootNodeSingleElementChainShowsItself covers the
// Finding-2 gap: a root node's ancestor chain has exactly one element (the
// node itself). The reversal loop in formatNodeDetail (leaf→root input,
// root→leaf output) must handle len==1 without an empty path, a doubled
// entry, or a stray " / " separator with nothing on one side.
func TestFormatNodeDetail_RootNodeSingleElementChainShowsItself(t *testing.T) {
	out := formatNodeDetail(nodeDetail{
		Node:  domain.Node{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		Chain: []domain.Node{{ID: "e1", Name: "Alpha"}},
	})
	if !strings.Contains(out, "path: Alpha") {
		t.Errorf("a root's chain must show itself as the whole path:\n%s", out)
	}
	if strings.Contains(out, "path: /") || strings.Contains(out, "path: \n") {
		t.Errorf("a single-element chain must not print an empty path:\n%s", out)
	}
	if strings.Contains(out, "Alpha / Alpha") {
		t.Errorf("a single-element chain must not be doubled:\n%s", out)
	}
	if strings.Contains(out, "Alpha /\n") || strings.Contains(out, "Alpha / \n") {
		t.Errorf("a single-element chain must not print a dangling separator:\n%s", out)
	}
}

func TestFormatNodeTags(t *testing.T) {
	node := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	out := formatNodeTags(node, []domain.Tag{{Slug: "go"}, {Slug: "audio"}})
	for _, want := range []string{"Jukebox", "jukebox", "go", "audio", "now has"} {
		if !strings.Contains(out, want) {
			t.Errorf("tag result missing %q in: %s", want, out)
		}
	}
	empty := formatNodeTags(node, nil)
	if !strings.Contains(empty, "no tags") {
		t.Errorf("cleared tags = %q, want it to state the empty result", empty)
	}
}

func TestBindingsForNode_FiltersClientSide(t *testing.T) {
	all := []domain.ProjectBinding{
		{ID: "b1", NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "a/b"},
		{ID: "b2", NodeID: "other", Kind: domain.BindingPath, MachineID: "m1", Path: "/x"},
		{ID: "b3", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m2", Path: "/y"},
	}
	got := bindingsForNode(all, "r1")
	if len(got) != 2 || got[0].ID != "b1" || got[1].ID != "b3" {
		t.Fatalf("bindingsForNode = %+v, want b1 and b3 in order", got)
	}
	if bindingsForNode(all, "nobody") != nil {
		t.Error("no match must yield nil, not an empty non-nil slice")
	}
}

func TestFormatBindingRows_ShowsMachineLabelAndID(t *testing.T) {
	rows := []bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "r1", Kind: domain.BindingRemote,
			RemoteSlug: "github.com/serverkraken/jukebox"}, NodeName: "Jukebox", NodeSlug: "jukebox"},
		{Binding: domain.ProjectBinding{ID: "b2", NodeID: "e1", Kind: domain.BindingPath,
			MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"}, NodeName: "Alpha", NodeSlug: "alpha"},
	}
	out := formatBindingRows(rows, "for this owner across all devices")
	for _, want := range []string{"2 binding", "github.com/serverkraken/jukebox", "Jukebox",
		"/work/alpha", "notebook-b", "m2", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("binding list missing %q in:\n%s", want, out)
		}
	}
	if empty := formatBindingRows(nil, "for node jukebox"); !strings.HasPrefix(empty, "No bindings") {
		t.Errorf("empty list = %q, want a 'No bindings' message", empty)
	}
}

// TestFormatBindingRows_EmptyStatePointsAtTheFixingTool is Finding 3 of the
// whole-branch review: every other empty state in this file names the tool
// that fixes it (formatNodeTree → flow_create_node, formatNodeTags → -,
// formatNodeDetail's empty tags → flow_set_node_tags, its empty bindings →
// flow_bind_project, an unresolved flow_node_binding target →
// flow_node_binding action="bind"). formatBindingRows' empty state — reached
// by the everyday flow_node_binding{action:"list"} call — used to name one
// too and regressed to a dead end.
func TestFormatBindingRows_EmptyStatePointsAtTheFixingTool(t *testing.T) {
	empty := formatBindingRows(nil, "for node jukebox")
	if !strings.Contains(empty, "flow_node_binding") && !strings.Contains(empty, "flow_bind_project") {
		t.Errorf("empty bindings must name a tool that creates a binding:\n%s", empty)
	}
}

func TestFormatBindingRows_UnknownNodeIsLabelledNotBlank(t *testing.T) {
	out := formatBindingRows([]bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "gone", Kind: domain.BindingRemote, RemoteSlug: "a/b"},
			NodeName: "(unknown node)", NodeSlug: "gone"},
	}, "for this owner")
	if !strings.Contains(out, "(unknown node)") || !strings.Contains(out, "gone") {
		t.Errorf("a binding whose node is not in the cache must still be identifiable:\n%s", out)
	}
}

// TestFormatBindingRowsAndResolve_LongPathsPassThroughVerbatim: a binding path is
// an address, so it is never shortened — same contract as the tree renderer.
func TestFormatBindingRowsAndResolve_LongPathsPassThroughVerbatim(t *testing.T) {
	longPath := "/Users/dev/" + strings.Repeat("sehr/tief/verschachtelt/", 12) + "repo"
	out := formatBindingRows([]bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "r1", Kind: domain.BindingPath,
			MachineID: "m1", MachineLabel: "notebook-a", Path: longPath}, NodeName: "Jukebox", NodeSlug: "jukebox"},
	}, "for node jukebox")
	if !strings.Contains(out, longPath) {
		t.Errorf("a long binding path must not be truncated:\n%s", out)
	}

	resolved := formatResolvedTarget("path "+longPath,
		domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo},
		domain.Node{}, false)
	if !strings.Contains(resolved, longPath) {
		t.Errorf("resolve must echo the full target:\n%s", resolved)
	}
}

func TestFormatResolvedTarget(t *testing.T) {
	node := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	eng := domain.Node{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement}

	withEng := formatResolvedTarget("path /work/jukebox", node, eng, true)
	for _, want := range []string{"/work/jukebox", "resolves to", "Jukebox", "jukebox", "r1", "Alpha", "alpha"} {
		if !strings.Contains(withEng, want) {
			t.Errorf("resolve result missing %q in: %s", want, withEng)
		}
	}
	without := formatResolvedTarget("remote a/b", node, domain.Node{}, false)
	if !strings.Contains(without, "No engagement") {
		t.Errorf("missing engagement must be stated, got: %s", without)
	}
}
