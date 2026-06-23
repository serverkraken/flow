// Package projectresolve runs the client-side project resolution chain.
package projectresolve

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientcheckout"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/serverkraken/flow/internal/gitworktree"
)

// recordCheckout records a resolved git repo's slug→root on this machine. A
// package var so tests can stub it (the real impl writes to the user config dir).
var recordCheckout = clientcheckout.Record

// SetRecordCheckoutForTest swaps the record hook and returns a restore func.
func SetRecordCheckoutForTest(f func(slug, root string) error) func() {
	prev := recordCheckout
	recordCheckout = f
	return func() { recordCheckout = prev }
}

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
	if remoteSlug != "" {
		if root, ok, _ := gitworktree.Root(cwd); ok {
			_ = recordCheckout(remoteSlug, root) // non-fatal
		}
	}

	m, err := clientmachine.Load()
	if err != nil {
		// Non-fatal: remote-slug tier still works without a machine id.
		m.ID = ""
	}

	return c.ResolveProject(ctx, remoteSlug, m.ID, filepath.Clean(cwd))
}
