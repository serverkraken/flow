package main

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// resolveSlug returns the node ID for the given ID, slug, or slash-path. A bare slug
// resolves when unambiguous; if several nodes share it, the error lists the
// fully-qualified paths to re-issue (slugs are unique only per sibling set).
func resolveSlug(ctx context.Context, c *apiclient.Client, slug string) (string, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	return resolveNodeRef(nodes, slug)
}
