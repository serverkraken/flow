package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// nodeTree is a node plus its (name-sorted) children.
type nodeTree struct {
	Node     domain.Node
	Children []*nodeTree
}

// buildTree groups a flat node list into a parent→children forest. A node whose
// ParentID is nil or points to an absent node is treated as a root. Roots and
// each child level are sorted by name (case-insensitive), with true roots
// (ParentID == nil) coming before orphans (ParentID != nil but parent absent).
func buildTree(nodes []domain.Node) []*nodeTree {
	byID := make(map[string]*nodeTree, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodeTree{Node: nodes[i]}
	}
	var trueRoots, orphans []*nodeTree
	for _, t := range byID {
		pid := t.Node.ParentID
		if pid == nil {
			trueRoots = append(trueRoots, t)
			continue
		}
		if parent, ok := byID[*pid]; ok {
			parent.Children = append(parent.Children, t)
		} else {
			orphans = append(orphans, t) // dangling parent → surface as root later
		}
	}
	var sortRec func(ts []*nodeTree)
	sortRec = func(ts []*nodeTree) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(ts[i].Node.Name) < strings.ToLower(ts[j].Node.Name)
		})
		for _, t := range ts {
			sortRec(t.Children)
		}
	}
	sortRec(trueRoots)
	sortRec(orphans)
	roots := append(trueRoots, orphans...)
	return roots
}

// renderTree writes the forest indented two spaces per depth level.
func renderTree(roots []*nodeTree, w io.Writer) {
	var walk func(ts []*nodeTree, depth int)
	walk = func(ts []*nodeTree, depth int) {
		for _, t := range ts {
			_, _ = fmt.Fprintf(w, "%s%s  %s (%s)\n", strings.Repeat("  ", depth), t.Node.Kind, t.Node.Name, t.Node.Slug)
			walk(t.Children, depth+1)
		}
	}
	walk(roots, 0)
}
