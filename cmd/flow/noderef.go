package main

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/noderef"
)

// resolveNodeRef resolves a bare slug or a slash-separated path (engagement/vorhaben/repo)
// against a flat node list. A bare slug resolves only when unambiguous; if several
// nodes share it (now possible — slugs are unique only per sibling set), it returns
// an error listing each match's fully-qualified path so the caller can re-issue one.
func resolveNodeRef(nodes []domain.Node, ref string) (string, error) {
	node, err := noderef.Resolve(nodes, ref)
	if err != nil {
		return "", err
	}
	return node.ID, nil
}
