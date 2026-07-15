// Package nodetree is the "Knoten" tab: an indented Engagement→Vorhaben→Repo
// hierarchy tree with kind + fuzzy filters, cursor nav, SSE live reload, and
// in-route move/delete dialogs. detail and form are pushed child routes.
package nodetree

import (
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

// Row is one rendered tree line: a node plus its depth (0 = engagement root).
type Row struct {
	Node  domain.Node
	Depth int
}

// BuildTree flattens nodes in DFS pre-order: each root (parent_id nil, or a
// parent absent from the set) followed by its descendants, every sibling group
// name-sorted (then ID for stability). Pure.
func BuildTree(nodes []domain.Node) []Row {
	present := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		present[n.ID] = true
	}
	byParent := map[string][]domain.Node{}
	for _, n := range nodes {
		key := ""
		if n.ParentID != nil && present[*n.ParentID] {
			key = *n.ParentID
		}
		byParent[key] = append(byParent[key], n)
	}
	for k := range byParent {
		kids := byParent[k]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].Name != kids[j].Name {
				return kids[i].Name < kids[j].Name
			}
			return kids[i].ID < kids[j].ID
		})
		byParent[k] = kids
	}
	var rows []Row
	seen := make(map[string]bool, len(nodes))
	var walkNode func(domain.Node, int)
	walkNode = func(n domain.Node, depth int) {
		if seen[n.ID] {
			return
		}
		seen[n.ID] = true
		rows = append(rows, Row{Node: n, Depth: depth})
		for _, child := range byParent[n.ID] {
			walkNode(child, depth+1)
		}
	}
	for _, root := range byParent[""] {
		walkNode(root, 0)
	}
	// A corrupt parent cycle has no root. Surface each remaining component at
	// level zero, deterministically, and let seen stop the back-edge.
	remaining := append([]domain.Node(nil), nodes...)
	sort.SliceStable(remaining, func(i, j int) bool {
		if remaining[i].Name != remaining[j].Name {
			return remaining[i].Name < remaining[j].Name
		}
		return remaining[i].ID < remaining[j].ID
	})
	for _, n := range remaining {
		walkNode(n, 0)
	}
	return rows
}

// FilterKind keeps rows whose node kind == kind; the zero kind keeps all. A
// non-empty kind flattens the result (Depth reset to 0) since ancestors are
// dropped. Pure.
func FilterKind(rows []Row, kind domain.NodeKind) []Row {
	if kind == "" {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Node.Kind == kind {
			out = append(out, Row{Node: r.Node, Depth: 0})
		}
	}
	return out
}

// FuzzyFilter keeps rows whose node name matches query (subsequence, via
// ui/fuzzymatch) PLUS every ancestor of a match, so the tree stays legible.
// Blank query is a no-op. Pure.
func FuzzyFilter(rows []Row, query string) []Row {
	if strings.TrimSpace(query) == "" {
		return rows
	}
	idx := make(map[string]int, len(rows))
	for i, r := range rows {
		idx[r.Node.ID] = i
	}
	keep := map[string]bool{}
	for _, r := range rows {
		if _, _, ok := fuzzymatch.Match(query, r.Node.Name); ok {
			keep[r.Node.ID] = true
			cur := r.Node
			seen := map[string]bool{cur.ID: true}
			for cur.ParentID != nil {
				if seen[*cur.ParentID] {
					break
				}
				seen[*cur.ParentID] = true
				j, ok := idx[*cur.ParentID]
				if !ok {
					break
				}
				keep[*cur.ParentID] = true
				cur = rows[j].Node
			}
		}
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if keep[r.Node.ID] {
			out = append(out, r)
		}
	}
	return out
}

// MoveCandidates returns the nodes a node may be reparented under: kind-valid
// (domain.ValidParentKind) and outside the node's own subtree (no cycle). The
// node itself is excluded; result name-sorted. Pure.
func MoveCandidates(all []domain.Node, node domain.Node) []domain.Node {
	inSubtree := map[string]bool{node.ID: true}
	for changed := true; changed; {
		changed = false
		for _, n := range all {
			if n.ParentID != nil && inSubtree[*n.ParentID] && !inSubtree[n.ID] {
				inSubtree[n.ID] = true
				changed = true
			}
		}
	}
	var out []domain.Node
	for _, n := range all {
		if inSubtree[n.ID] {
			continue
		}
		if domain.ValidParentKind(node.Kind, n.Kind) {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
