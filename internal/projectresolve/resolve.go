// Package projectresolve runs the client-side project resolution chain.
package projectresolve

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
)

// Resolve answers "which project is cwd?" using a Slice-1 precedence chain:
//
//  1. FLOW_PROJECT env var → match against ListProjects by slug; not found → error.
//  2. git remote slug (OriginSlug) → server ResolveProject (machineID empty in Slice 1).
//
// getenv and cwd are injected to keep the function pure/testable.
func Resolve(ctx context.Context, c *apiclient.Client, getenv func(string) string, cwd string) (domain.Project, bool, error) {
	if slug := getenv("FLOW_PROJECT"); slug != "" {
		projects, err := c.ListProjects(ctx)
		if err != nil {
			return domain.Project{}, false, fmt.Errorf("projectresolve: list projects: %w", err)
		}
		for _, p := range projects {
			if p.Slug == slug {
				return p, true, nil
			}
		}
		return domain.Project{}, false, fmt.Errorf("projectresolve: unknown FLOW_PROJECT slug %q", slug)
	}

	remoteSlug, _, _ := gitremote.OriginSlug(cwd)
	return c.ResolveProject(ctx, remoteSlug, "", cwd)
}
