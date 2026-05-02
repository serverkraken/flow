package gitsnapshot

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// GetRemote implements ports.NotebookRemote.
func (m Manager) GetRemote(ctx context.Context, root string) (string, error) {
	out, err := m.run(ctx, root, "remote", "get-url", "origin")
	if err != nil {
		if isExitErr(err) {
			return "", ports.ErrNoRemoteConfigured
		}
		return "", fmt.Errorf("git remote get-url: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// SetRemote implements ports.NotebookRemote. Tries to update an existing
// origin first; falls back to `remote add` if origin is missing.
func (m Manager) SetRemote(ctx context.Context, root, url string) error {
	if _, err := m.run(ctx, root, "remote", "set-url", "origin", url); err == nil {
		return nil
	}
	if _, err := m.run(ctx, root, "remote", "add", "origin", url); err != nil {
		return fmt.Errorf("git remote add origin: %w", err)
	}
	return nil
}

// Sync implements ports.NotebookRemote.
//
// The flow is `git pull --rebase --autostash origin <branch>` followed
// by `git push --set-upstream origin <branch>`. --autostash
// transparently shelves any dirty working-tree changes around the pull
// so the user doesn't have to think about a clean tree before syncing
// — those changes stay local and unpushed (commit them with
// `kompendium snapshot` to make them travel).
//
// On a first-ever sync (the bare remote has no <branch> yet), the
// pull is skipped — pulling against a missing remote ref errors out,
// and there's nothing to fetch anyway. The subsequent push creates the
// branch on the remote and sets the upstream tracking ref.
//
// Identity injection (`-c user.name=…`) only fires when the host has
// no git identity configured — same fallback as commit/merge so real
// authors are preserved across machines.
func (m Manager) Sync(ctx context.Context, root string) (ports.SyncStats, error) {
	if _, err := m.GetRemote(ctx, root); err != nil {
		return ports.SyncStats{}, err
	}
	branch, err := m.currentBranch(ctx, root)
	if err != nil {
		return ports.SyncStats{}, fmt.Errorf("detect current branch: %w", err)
	}

	exists, err := m.remoteBranchExists(ctx, root, branch)
	if err != nil {
		return ports.SyncStats{}, fmt.Errorf("probe remote branch: %w", err)
	}

	stats := ports.SyncStats{}
	if exists {
		pullArgs := identityArgs(ctx, m.run, root, []string{
			"pull", "--rebase", "--autostash", "origin", branch,
		})
		if _, err := m.run(ctx, root, pullArgs...); err != nil {
			return stats, fmt.Errorf("git pull: %w", err)
		}
		stats.Pulled = true
	}

	if _, err := m.run(ctx, root, "push", "--set-upstream", "origin", branch); err != nil {
		return stats, fmt.Errorf("git push: %w", err)
	}
	stats.Pushed = true
	return stats, nil
}

// remoteBranchExists reports whether the remote has a head with the
// given branch name. Used by Sync to decide if the first step should
// be a pull (steady state) or just a push (first-ever sync).
func (m Manager) remoteBranchExists(ctx context.Context, root, branch string) (bool, error) {
	out, err := m.run(ctx, root, "ls-remote", "--heads", "origin", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
