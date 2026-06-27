package main

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// resolveSlug returns the project ID for the given slug, or an error if none matches.
func resolveSlug(ctx context.Context, c *apiclient.Client, slug string) (string, error) {
	projects, err := c.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		if p.Slug == slug {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("no project with slug %q", slug)
}
