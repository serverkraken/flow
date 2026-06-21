// Package projectresolve runs the client-side project resolution chain.
package projectresolve

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
)

// Resolve answers "which project is cwd?" using a precedence chain:
//
//  1. FLOW_PROJECT env var → match against ListProjects by slug; not found → error.
//  2. git remote slug (OriginSlug) + machine id + cleaned cwd → server ResolveProject.
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

	m, err := clientmachine.Load()
	if err != nil {
		// Non-fatal: remote-slug tier still works without a machine id.
		m.ID = ""
	}

	return c.ResolveProject(ctx, remoteSlug, m.ID, filepath.Clean(cwd))
}
