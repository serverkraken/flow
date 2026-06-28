package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// resolveNodeRef resolves a bare slug or a slash-separated path (engagement/vorhaben/repo)
// against a flat node list. A bare slug resolves only when unambiguous; if several
// nodes share it (now possible — slugs are unique only per sibling set), it returns
// an error listing each match's fully-qualified path so the caller can re-issue one.
func resolveNodeRef(nodes []domain.Node, ref string) (string, error) {
	if strings.Contains(ref, "/") {
		return resolveNodePath(nodes, ref)
	}
	var matches []domain.Node
	for _, n := range nodes {
		if n.Slug == ref {
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no node with slug %q", ref)
	case 1:
		return matches[0].ID, nil
	default:
		byID := indexByID(nodes)
		paths := make([]string, 0, len(matches))
		for _, m := range matches {
			paths = append(paths, nodePathOf(byID, m))
		}
		sort.Strings(paths)
		return "", fmt.Errorf("slug %q is ambiguous; qualify with a path: %s", ref, strings.Join(paths, ", "))
	}
}

// resolveNodePath walks a slash-separated path from a root engagement down to a
// leaf, matching each segment against the children of the previous node by slug.
func resolveNodePath(nodes []domain.Node, ref string) (string, error) {
	segs := make([]string, 0)
	for _, s := range strings.Split(ref, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "", fmt.Errorf("empty node path %q", ref)
	}
	var parentID *string // nil = root level
	var curID string
	for i, seg := range segs {
		child, ok := findChildBySlug(nodes, parentID, seg)
		if !ok {
			return "", fmt.Errorf("no node %q under %q", seg, strings.Join(segs[:i], "/"))
		}
		curID = child.ID
		parentID = &child.ID
	}
	return curID, nil
}

// findChildBySlug finds the node whose parent is parentID (nil = a root) and whose
// slug is slug. Per-sibling uniqueness guarantees at most one match.
func findChildBySlug(nodes []domain.Node, parentID *string, slug string) (domain.Node, bool) {
	for _, n := range nodes {
		if n.Slug != slug {
			continue
		}
		if (n.ParentID == nil) == (parentID == nil) && (parentID == nil || *n.ParentID == *parentID) {
			return n, true
		}
	}
	return domain.Node{}, false
}

func indexByID(nodes []domain.Node) map[string]domain.Node {
	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	return byID
}

// nodePathOf builds a node's fully-qualified slug path (root→leaf, slash-joined).
func nodePathOf(byID map[string]domain.Node, n domain.Node) string {
	var parts []string
	cur := n
	for {
		parts = append([]string{cur.Slug}, parts...)
		if cur.ParentID == nil {
			break
		}
		parent, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		cur = parent
	}
	return strings.Join(parts, "/")
}
