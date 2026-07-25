package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// nodeKindGlyph maps a node kind to a monospace glyph. AGENTS.md bans emoji
// pictograms and sanctions ● ◆ ⬡ ▶ ■. Every line also prints the kind word, so
// the glyph is a scanning aid and never information a model has to decode.
func nodeKindGlyph(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "●"
	case domain.KindVorhaben:
		return "◆"
	case domain.KindRepo:
		return "⬡"
	case domain.KindBranch:
		return "▶"
	default:
		return "·"
	}
}

// nodeTreeEntry is a node plus its name-sorted children.
type nodeTreeEntry struct {
	Node     domain.Node
	Children []*nodeTreeEntry
}

// buildNodeForest groups a flat node list into a parent→children forest. A node
// whose ParentID is nil is a root; a node whose ParentID points at an ID the
// list does not contain becomes a root too, rather than vanishing — hiding a
// node the owner does own would be worse than showing it unindented. Roots and
// every child level are name-sorted case-insensitively, true roots before
// dangling ones. Acyclicity is a server invariant (MoveNode rejects cycles with
// usecase.ErrNodeCycle), so the recursion always terminates.
func buildNodeForest(nodes []domain.Node) []*nodeTreeEntry {
	byID := make(map[string]*nodeTreeEntry, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodeTreeEntry{Node: nodes[i]}
	}
	var roots, dangling []*nodeTreeEntry
	// Iterate the slice, not the map, so the pre-sort order is deterministic.
	for i := range nodes {
		entry := byID[nodes[i].ID]
		pid := entry.Node.ParentID
		if pid == nil {
			roots = append(roots, entry)
			continue
		}
		if parent, ok := byID[*pid]; ok {
			parent.Children = append(parent.Children, entry)
			continue
		}
		dangling = append(dangling, entry)
	}
	var sortRec func(ts []*nodeTreeEntry)
	sortRec = func(ts []*nodeTreeEntry) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(ts[i].Node.Name) < strings.ToLower(ts[j].Node.Name)
		})
		for _, t := range ts {
			sortRec(t.Children)
		}
	}
	sortRec(roots)
	sortRec(dangling)
	return append(roots, dangling...)
}

// formatNodeTree renders the hierarchy indented two spaces per level, one line
// per node: kind glyph, name, slug, kind, status, id, and upstream when set.
// The flat alphabetical predecessor showed neither kind nor parent — exactly the
// information flow_create_node needs to pick a valid parent (Spec §3).
func formatNodeTree(nodes []domain.Node) string {
	if len(nodes) == 0 {
		return `No nodes yet. Create the first one with flow_create_node (kind="engagement", no parent).`
	}
	var b strings.Builder
	var walk func(ts []*nodeTreeEntry, depth int)
	walk = func(ts []*nodeTreeEntry, depth int) {
		for _, t := range ts {
			n := t.Node
			fmt.Fprintf(&b, "%s%s %s (%s) — %s — %s — %s",
				strings.Repeat("  ", depth), nodeKindGlyph(n.Kind), n.Name, n.Slug, n.Kind, n.Status, n.ID)
			if n.UpstreamGit != "" {
				fmt.Fprintf(&b, " — %s", n.UpstreamGit)
			}
			b.WriteByte('\n')
			walk(t.Children, depth+1)
		}
	}
	walk(buildNodeForest(nodes), 0)
	return strings.TrimRight(b.String(), "\n")
}
