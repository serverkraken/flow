package gitsnapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExportBundle implements ports.NotebookBundler.
func (m Manager) ExportBundle(ctx context.Context, root, outPath string) error {
	if _, err := m.run(ctx, root, "bundle", "create", outPath, "--all"); err != nil {
		return fmt.Errorf("git bundle create: %w", err)
	}
	return nil
}

// ImportBundle implements ports.NotebookBundler.
//
// Verifies the bundle, fetches every branch into a kompendium-bundle/*
// remote-tracking namespace so it can never overwrite local refs, merges
// the bundle's same-named branch into the current branch, then cleans up
// the kompendium-bundle/* refs so they don't accumulate across imports.
// Merge conflicts surface unchanged so the user resolves them with their
// editor.
func (m Manager) ImportBundle(ctx context.Context, root, bundlePath string) error {
	if _, err := m.run(ctx, root, "bundle", "verify", bundlePath); err != nil {
		return fmt.Errorf("git bundle verify: %w", err)
	}
	if _, err := m.run(
		ctx, root, "fetch", bundlePath,
		"+refs/heads/*:refs/remotes/kompendium-bundle/*",
	); err != nil {
		return fmt.Errorf("git fetch bundle: %w", err)
	}
	branch, err := m.currentBranch(ctx, root)
	if err != nil {
		return fmt.Errorf("detect current branch: %w", err)
	}
	mergeArgs := []string{
		"merge", "--no-edit", "--allow-unrelated-histories",
		"kompendium-bundle/" + branch,
	}
	if _, err := m.run(ctx, root, identityArgs(ctx, m.run, root, mergeArgs)...); err != nil {
		// Cleanup runs best-effort but its error is now joined to the
		// merge failure so a leaking ref accumulation is visible
		// instead of being lost to `_ =`. Without this, every failed
		// merge silently grew refs/remotes/kompendium-bundle/* — exactly
		// the leak the cleanup was meant to prevent.
		mergeErr := fmt.Errorf("git merge bundle: %w", err)
		if cerr := m.cleanupBundleRefs(ctx, root); cerr != nil {
			return errors.Join(mergeErr, fmt.Errorf("cleanup bundle refs: %w", cerr))
		}
		return mergeErr
	}
	if err := m.cleanupBundleRefs(ctx, root); err != nil {
		return fmt.Errorf("cleanup bundle refs: %w", err)
	}
	return nil
}

// cleanupBundleRefs removes every refs/remotes/kompendium-bundle/* ref so
// successive imports don't leave a growing trail of stale tracking refs.
// Errors are returned but only mid-import; post-merge they're best-effort.
func (m Manager) cleanupBundleRefs(ctx context.Context, root string) error {
	out, err := m.run(ctx, root, "for-each-ref",
		"--format=%(refname)", "refs/remotes/kompendium-bundle/")
	if err != nil {
		return fmt.Errorf("list bundle refs: %w", err)
	}
	for _, ref := range strings.Split(strings.TrimSpace(out), "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, err := m.run(ctx, root, "update-ref", "-d", ref); err != nil {
			return fmt.Errorf("delete ref %q: %w", ref, err)
		}
	}
	return nil
}
