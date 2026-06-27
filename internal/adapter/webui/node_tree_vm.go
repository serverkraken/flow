package webui

import (
	"fmt"
	"html/template"
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

// TreeRow is one rendered line of the node tree: the node plus its depth (0 =
// engagement root) so the template can indent it.
type TreeRow struct {
	Level int
	Node  domain.Node
}

// buildNodeTree turns a flat node slice into a depth-first, indented row list:
// engagement roots first, each followed by its subtree; siblings are ordered by
// name. It is cycle- and orphan-safe — any node whose parent is absent (or that
// would re-enter a visited subtree) is surfaced at level 0 so nothing is hidden.
func buildNodeTree(nodes []domain.Node) []TreeRow {
	children := map[string][]domain.Node{}
	for _, n := range nodes {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	for k := range children {
		sort.SliceStable(children[k], func(i, j int) bool {
			return children[k][i].Name < children[k][j].Name
		})
	}
	var out []TreeRow
	seen := map[string]bool{}
	var walk func(parentKey string, level int)
	walk = func(parentKey string, level int) {
		for _, n := range children[parentKey] {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			out = append(out, TreeRow{Level: level, Node: n})
			walk(n.ID, level+1)
		}
	}
	walk("", 0)
	// Orphans (parent not in the set) — defensive: never drop a node.
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, TreeRow{Level: 0, Node: n})
		}
	}
	return out
}

// ValidParentsFor returns the nodes that may host a child of the given kind,
// name-ordered. Engagement is always a root → empty result.
func ValidParentsFor(kind domain.NodeKind, all []domain.Node) []domain.Node {
	if kind == domain.KindEngagement {
		return nil
	}
	var out []domain.Node
	for _, n := range all {
		if domain.AllowedChildKind(n.Kind, kind) {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// descendantIDs returns id plus every node in its subtree. Move targets must be
// excluded from this set to keep reparenting acyclic.
func descendantIDs(all []domain.Node, id string) map[string]bool {
	children := map[string][]string{}
	for _, n := range all {
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	out := map[string]bool{id: true}
	var walk func(string)
	walk = func(cur string) {
		for _, c := range children[cur] {
			if !out[c] {
				out[c] = true
				walk(c)
			}
		}
	}
	walk(id)
	return out
}

// ---- page/form/cockpit view models (rendered by D3–D6) ----

// NodesPageData is the tree view model (node-management home).
type NodesPageData struct {
	User   string
	Status string // "" (active+paused) | "archived" | "all"
	Rows   []TreeRow
}

// NodeFormValues holds raw create/edit form fields (re-rendered on error).
type NodeFormValues struct {
	Name, Slug, Kind, ParentID      string
	Description, UpstreamGit, Status string
	Color, Glyph                    string
	RateAmount, RateCurrency        string
}

// NodeFormData drives the create (editing==nil) / edit form.
type NodeFormData struct {
	User    string
	Error   string
	Vals    NodeFormValues
	Parents []domain.Node // candidate parents (engagements + vorhaben)
}

// NodeCockpit is the read-only repo/vorhaben/engagement detail view model.
type NodeCockpit struct {
	User            string
	N               domain.Node
	Ancestors       []domain.Node // leaf→root (as NodeStore.Ancestors returns)
	DescriptionHTML template.HTML
	TotalHours      float64
	WeekHours       float64
	MonthHours      float64
	Earnings        string
	Docs            []domain.Document
	Bindings        []domain.ProjectBinding // valid bindings for inline display
	MoveTargets     []domain.Node           // valid new parents (for the inline move form)
}

// NodeMoveData drives the inline move form on the cockpit page.
type NodeMoveData struct {
	User    string
	N       domain.Node
	Targets []domain.Node // valid new parents (descendant IDs excluded)
}

// BuildTree is the exported entry point used by the httpserver adapter.
func BuildTree(nodes []domain.Node) []TreeRow { return buildNodeTree(nodes) }

// nodeFilterChip returns Tailwind chip classes for the filter bar; active = ink
// background, inactive = muted text with hover accent.
func nodeFilterChip(active bool) string {
	if active {
		return "rounded-full bg-ink px-3 py-1 text-xs font-medium text-canvas"
	}
	return "rounded-full border border-line bg-surface px-3 py-1 text-xs font-medium text-muted hover:border-blue/40 hover:text-blue"
}

// nodeIndentStyle returns an inline CSS padding-left for depth-based indentation
// in the tree (1 rem per level).
func nodeIndentStyle(level int) string { return fmt.Sprintf("padding-left:%drem", level) }
