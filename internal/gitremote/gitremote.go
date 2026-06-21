// Package gitremote resolves a directory's git origin remote into a normalized slug.
package gitremote

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// OriginSlug returns the normalized "host/path" slug for the git origin remote
// of the repository that owns dir (worktree-invariant: worktrees share origin).
//
// ok=false, err=nil means dir is not a git repo or has no origin remote.
// err!=nil means git itself could not be executed (e.g. binary not found).
//
// V0 choice: any non-zero git exit is treated as "not a repo / no origin"
// (ok=false, err=nil) rather than a hard error. This avoids brittle stderr
// parsing across git versions at the cost of hiding unusual git failures —
// acceptable for a client-side helper.
func OriginSlug(dir string) (slug string, ok bool, err error) {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, runErr := cmd.Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// git ran but returned non-zero: not a repo or no origin remote.
			return "", false, nil
		}
		// git binary itself could not be found or executed.
		return "", false, runErr
	}
	slug, ok = domain.NormalizeRemoteSlug(strings.TrimSpace(string(out)))
	return slug, ok, nil
}
