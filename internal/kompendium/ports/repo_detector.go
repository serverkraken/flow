package ports

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// ErrNotInRepo is returned when a RepoDetector cannot find an enclosing git
// repository for the given working directory.
var ErrNotInRepo = errors.New("not in a git repository")

// ErrRepoHasNoRemote is returned when the enclosing repository has no
// "origin" remote configured. Project notes need a stable canonical URL
// across machines, so this is a hard failure for CreateProject — local-
// only repos belong under `notes/` (free notes), not `projects/`.
var ErrRepoHasNoRemote = errors.New("repository has no origin remote")

// RepoInfo is the output of RepoDetector.Detect.
type RepoInfo struct {
	Root string
	URL  domain.CanonicalURL
}

// RepoDetector resolves a working directory to its enclosing git repository
// and that repository's canonical URL.
type RepoDetector interface {
	Detect(ctx context.Context, cwd string) (RepoInfo, error)
}
