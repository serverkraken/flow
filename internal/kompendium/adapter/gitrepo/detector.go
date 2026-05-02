package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Detector resolves a working directory to its enclosing git repository.
// Construct it via New(); the zero value is not usable.
type Detector struct {
	run runFunc
}

// runFunc abstracts shelling out to git so tests can inject failure modes
// (notably non-exit errors like a missing git binary) that real git
// invocations cannot easily reproduce.
type runFunc func(ctx context.Context, dir string, args ...string) (string, error)

// New returns a Detector backed by the real git binary on $PATH.
func New() Detector {
	return Detector{run: realRun}
}

// Detect implements ports.RepoDetector.
//
// A missing repository or non-existent cwd surfaces as ports.ErrNotInRepo.
// A repository present but without an "origin" remote surfaces as
// ports.ErrRepoHasNoRemote — project notes need a stable canonical URL
// across machines, and the previous filesystem-path fallback produced
// keys that broke as soon as the user moved to a second machine.
func (d Detector) Detect(ctx context.Context, cwd string) (ports.RepoInfo, error) {
	root, err := d.run(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		if isExitErr(err) {
			return ports.RepoInfo{}, ports.ErrNotInRepo
		}
		return ports.RepoInfo{}, fmt.Errorf("git rev-parse: %w", err)
	}

	rawURL, urlErr := d.run(ctx, root, "remote", "get-url", "origin")
	if urlErr != nil {
		if isExitErr(urlErr) {
			// Root is still useful (callers may want to know the repo
			// boundary even when no project ID can be derived), so we
			// return it alongside the sentinel.
			return ports.RepoInfo{Root: root}, ports.ErrRepoHasNoRemote
		}
		return ports.RepoInfo{}, fmt.Errorf("git remote get-url: %w", urlErr)
	}

	return ports.RepoInfo{
		Root: root,
		URL:  domain.NormalizeURL(rawURL),
	}, nil
}

func realRun(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isExitErr(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

var _ ports.RepoDetector = Detector{}
