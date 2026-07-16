// Package noderef resolves owner-scoped node lists by stable ID, unique bare
// slug, or a fully-qualified root-to-leaf slug path.
package noderef

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

var (
	ErrNotFound  = errors.New("node reference not found")
	ErrAmbiguous = errors.New("node reference is ambiguous")
)

// Resolve returns exactly one node. Stable IDs win over human-readable
// references; bare slugs must be owner-wide unique, while qualified paths use
// the sibling-unique hierarchy.
func Resolve(nodes []domain.Node, ref string) (domain.Node, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return domain.Node{}, fmt.Errorf("%w: empty node reference", ErrNotFound)
	}
	for _, n := range nodes {
		if n.ID == ref {
			return n, nil
		}
	}
	if strings.Contains(ref, "/") {
		return resolvePath(nodes, ref)
	}

	var matches []domain.Node
	for _, n := range nodes {
		if strings.EqualFold(n.Slug, ref) {
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 0:
		return domain.Node{}, fmt.Errorf("%w: no node with slug %q", ErrNotFound, ref)
	case 1:
		return matches[0], nil
	default:
		byID := indexByID(nodes)
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, pathOf(byID, match))
		}
		sort.Strings(paths)
		return domain.Node{}, fmt.Errorf("%w: slug %q; qualify with a path: %s", ErrAmbiguous, ref, strings.Join(paths, ", "))
	}
}

func resolvePath(nodes []domain.Node, ref string) (domain.Node, error) {
	segments := make([]string, 0)
	for _, segment := range strings.Split(ref, "/") {
		if segment = strings.TrimSpace(segment); segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return domain.Node{}, fmt.Errorf("%w: empty node path %q", ErrNotFound, ref)
	}

	var parentID *string
	var current domain.Node
	for i, segment := range segments {
		found := false
		for _, n := range nodes {
			if !strings.EqualFold(n.Slug, segment) || !sameParent(n.ParentID, parentID) {
				continue
			}
			current = n
			found = true
			break
		}
		if !found {
			return domain.Node{}, fmt.Errorf("%w: no node %q under %q", ErrNotFound, segment, strings.Join(segments[:i], "/"))
		}
		parentID = &current.ID
	}
	return current, nil
}

func sameParent(a, b *string) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func indexByID(nodes []domain.Node) map[string]domain.Node {
	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	return byID
}

// QualifiedPath returns a cycle-safe root-to-node slug path for display and
// for disambiguating sibling-local slugs.
func QualifiedPath(nodes []domain.Node, node domain.Node) string {
	return pathOf(indexByID(nodes), node)
}

func pathOf(byID map[string]domain.Node, n domain.Node) string {
	parts := make([]string, 0)
	seen := make(map[string]bool, len(byID))
	current := n
	for {
		if seen[current.ID] {
			parts = append([]string{"…"}, parts...)
			break
		}
		seen[current.ID] = true
		parts = append([]string{current.Slug}, parts...)
		if current.ParentID == nil {
			break
		}
		parent, ok := byID[*current.ParentID]
		if !ok {
			break
		}
		current = parent
	}
	return strings.Join(parts, "/")
}
