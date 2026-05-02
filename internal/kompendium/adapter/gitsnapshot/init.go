package gitsnapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// defaultGitignore is the .gitignore kompendium drops into a freshly
// initialised notebook. The patterns target the artifacts that
// macOS/Linux editors and the OS itself routinely scatter in working
// trees — without them, `kompendium snapshot` would happily commit
// .DS_Store and *.swp to the synced notebook.
const defaultGitignore = `.DS_Store
.AppleDouble
.LSOverride
*.swp
*.swo
*.swn
*~
.idea/
.vscode/
.kompendium-*.tmp
`

// IsRepo implements ports.NotebookInitializer.
func (m Manager) IsRepo(ctx context.Context, root string) (bool, error) {
	out, err := m.run(ctx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if isExitErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(out) == "true", nil
}

// Init implements ports.NotebookInitializer.
func (m Manager) Init(ctx context.Context, root string) error {
	if _, err := m.run(ctx, root, "init", "-q", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := writeGitignoreIfMissing(root); err != nil {
		return err
	}
	if _, err := m.run(ctx, root, "add", "."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := m.commit(ctx, root, "kompendium notebook initialized"); err != nil {
		return err
	}
	return nil
}

// writeGitignoreIfMissing seeds a default .gitignore when none exists.
// We never overwrite an existing file — if the user already curated
// their own ignore patterns, those win.
func writeGitignoreIfMissing(root string) error {
	path := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil // user already has one; respect it
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(defaultGitignore), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// HasUncommittedChanges implements ports.NotebookInitializer.
func (m Manager) HasUncommittedChanges(ctx context.Context, root string) (bool, error) {
	out, err := m.run(ctx, root, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// Snapshot implements ports.NotebookInitializer.
func (m Manager) Snapshot(ctx context.Context, root, message string) error {
	if _, err := m.run(ctx, root, "add", "."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := m.commit(ctx, root, message); err != nil {
		return err
	}
	return nil
}

// commit invokes `git commit`, injecting a kompendium fallback identity
// only when the host has no user.name/user.email configured. The
// `--allow-empty` flag keeps Snapshot idempotent even on clean trees,
// which matters for callers that snapshot before exporting a bundle.
func (m Manager) commit(ctx context.Context, root, message string) error {
	args := identityArgs(ctx, m.run, root, []string{
		"commit", "--allow-empty", "-q", "-m", message,
	})
	if _, err := m.run(ctx, root, args...); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}
