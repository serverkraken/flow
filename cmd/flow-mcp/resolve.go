package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/projectresolve"
)

// resolveProject answers "which flow project is this directory?" via the V0
// resolution chain (FLOW_PROJECT override → git remote → per-device path).
// Any failure degrades to "no project" (matched=false) rather than erroring.
func resolveProject(ctx context.Context, client *apiclient.Client, log *slog.Logger) (domain.Project, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Warn("cannot determine working directory; no project scope", "err", err)
		return domain.Project{}, false
	}
	proj, matched, err := projectresolve.Resolve(ctx, client, os.Getenv, cwd)
	if err != nil {
		log.Warn("project resolution failed; no project scope", "err", err)
		return domain.Project{}, false
	}
	return proj, matched
}
