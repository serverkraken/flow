package gitsnapshot_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/gitsnapshot"
)

// TestIdentity_FallbackWhenUnconfigured covers a tempdir repo with no
// global identity inheritance — the fallback "kompendium <kompendium@local>"
// must show up in the commit so init/snapshot don't fail with
// "Please tell me who you are."
func TestIdentity_FallbackWhenUnconfigured(t *testing.T) {
	// t.Setenv forbids t.Parallel — keep these serial.
	m := gitsnapshot.New()
	ctx := context.Background()

	tmp := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}

	out := mustGitOutput(t, tmp, "log", "-1", "--format=%an <%ae>")
	if !strings.Contains(out, "kompendium") || !strings.Contains(out, "kompendium@local") {
		t.Errorf("fallback identity missing from commit: %q", out)
	}
}

// TestIdentity_PreservedWhenConfigured covers the regression we fixed:
// a host-side identity must survive into kompendium snapshots.
// Otherwise sync between two machines anonymises every commit.
func TestIdentity_PreservedWhenConfigured(t *testing.T) {
	// t.Setenv forbids t.Parallel — keep these serial.
	m := gitsnapshot.New()
	ctx := context.Background()

	tmp := newRepoWithoutIdentity(t)
	// Local config can only be set after `git init`, so init first.
	if err := m.Init(ctx, tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}
	mustGit(t, tmp, "config", "user.name", "Test User")
	mustGit(t, tmp, "config", "user.email", "user@example.test")

	if err := os.WriteFile(filepath.Join(tmp, "n.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, tmp, "snap"); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	out := mustGitOutput(t, tmp, "log", "-1", "--format=%an <%ae>")
	if !strings.Contains(out, "Test User") || !strings.Contains(out, "user@example.test") {
		t.Errorf("configured identity should be preserved, got %q", out)
	}
	if strings.Contains(out, "kompendium@local") {
		t.Errorf("kompendium fallback must not override real identity, got %q", out)
	}
}

// TestImportBundle_RespectsCurrentBranch covers the previously hardcoded
// `kompendium-bundle/main` merge target. Both sides operate on `main`
// here (gitsnapshot.Init uses `git init -b main`), so the test would
// also pass with the old code — but it locks in the contract: the
// merge target tracks HEAD, not a literal "main".
func TestImportBundle_RespectsCurrentBranch(t *testing.T) {
	// t.Setenv forbids t.Parallel — keep these serial.
	m := gitsnapshot.New()
	ctx := context.Background()

	src := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "x.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, src, "src x"); err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(t.TempDir(), "snap.bundle")
	if err := m.ExportBundle(ctx, src, bundlePath); err != nil {
		t.Fatal(err)
	}

	dst := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, dst); err != nil {
		t.Fatal(err)
	}
	if err := m.ImportBundle(ctx, dst, bundlePath); err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}

	// Post-merge, the bundle's content must be present.
	if _, err := os.Stat(filepath.Join(dst, "x.md")); err != nil {
		t.Errorf("bundle content should be merged, got: %v", err)
	}
	// And the kompendium-bundle/* refs must be cleaned up so successive
	// imports don't accumulate stale tracking refs.
	refs := mustGitOutput(t, dst, "for-each-ref", "--format=%(refname)",
		"refs/remotes/kompendium-bundle/")
	if strings.TrimSpace(refs) != "" {
		t.Errorf("kompendium-bundle/* refs should be cleaned up, got %q", refs)
	}
}

// --- helpers ----------------------------------------------------------------

// newRepoWithoutIdentity returns a fresh tempdir whose git operations
// resolve to *no* identity from any source — global ~/.gitconfig,
// system /etc/gitconfig, or GIT_AUTHOR_*/GIT_COMMITTER_*/EMAIL env vars.
// All three layers must be neutralised, otherwise the developer's
// ambient git identity leaks into the test and the kompendium fallback
// silently never runs.
func newRepoWithoutIdentity(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	cfgHome := t.TempDir()

	// Disable global + system config files via well-known git env vars.
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(cfgHome, "no-such-gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	// Identity env vars must be UNSET (not "") — git treats an empty
	// GIT_AUTHOR_NAME as "set but invalid" and refuses the commit. Use
	// os.Unsetenv with manual restore so we get the unset semantics.
	unsetEnv(
		t,
		"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE",
		"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE",
		"EMAIL",
	)
	return tmp
}

// unsetEnv removes the given env vars for the duration of the test and
// restores them afterwards. Necessary because t.Setenv can only set, not
// unset, and "" has different semantics from absent for some tools.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	saved := map[string]string{}
	had := map[string]bool{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
			had[k] = true
		}
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if had[k] {
				_ = os.Setenv(k, saved[k])
			} else {
				_ = os.Unsetenv(k)
			}
		}
	})
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
