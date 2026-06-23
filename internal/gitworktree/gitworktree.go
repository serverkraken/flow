// Package gitworktree reads a repo's worktrees live via the git binary. It is
// READ-ONLY: it never runs write-side git (no add/rm/clone). Mirrors
// internal/gitremote (same exec/error shape).
package gitworktree

import (
	"errors"
	"os/exec"
	"strings"
)

// Worktree is one entry of `git worktree list`. Branch is the short branch name,
// or "" when detached (HeadShort still carries the commit). IsMain marks the
// primary worktree (the first porcelain block).
type Worktree struct {
	Path      string
	Branch    string
	HeadShort string
	Dirty     bool
	IsMain    bool
}

// Root returns dir's worktree top level. ok=false (err=nil) when dir is not a
// git repo. A real error (git missing) is returned as err.
func Root(dir string) (root string, ok bool, err error) {
	out, runErr := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return "", false, nil
		}
		return "", false, runErr
	}
	return strings.TrimSpace(string(out)), true, nil
}

// List returns all worktrees of the repo containing root (git lists every
// worktree from any one of them). The slice is in git's order; the first entry
// is the main worktree.
func List(root string) ([]Worktree, error) {
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	var wts []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			cur.Dirty = isDirty(cur.Path)
			wts = append(wts, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree "), IsMain: len(wts) == 0}
		case cur == nil:
			// skip
		case strings.HasPrefix(line, "HEAD "):
			sha := strings.TrimPrefix(line, "HEAD ")
			if len(sha) > 7 {
				sha = sha[:7]
			}
			cur.HeadShort = sha
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Branch = "" // detached: Branch empty, HeadShort carries identity
		}
	}
	flush()
	return wts, nil
}

// isDirty reports whether the worktree at path has uncommitted tracked changes
// (cheap: ignores untracked files via -uno). Errors → treated as clean.
func isDirty(path string) bool {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain", "-uno").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
