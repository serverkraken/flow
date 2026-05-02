package gitrepo_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/gitrepo"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestDetector_NotInRepo(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	d := gitrepo.New()

	_, err := d.Detect(context.Background(), tmp)
	if !errors.Is(err, ports.ErrNotInRepo) {
		t.Errorf("got %v, want ErrNotInRepo", err)
	}
}

func TestDetector_NonexistentCwd(t *testing.T) {
	t.Parallel()
	d := gitrepo.New()

	_, err := d.Detect(context.Background(), "/nonexistent-path-for-test-only")
	if !errors.Is(err, ports.ErrNotInRepo) {
		t.Errorf("got %v, want ErrNotInRepo", err)
	}
}

func TestDetector_RepoWithRemote(t *testing.T) {
	t.Parallel()
	tmp := initRepo(t, "git@github.com:Foo/Bar.git")
	d := gitrepo.New()

	info, err := d.Detect(context.Background(), tmp)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.URL != domain.CanonicalURL("github.com/foo/bar") {
		t.Errorf("URL got %q, want %q", info.URL, "github.com/foo/bar")
	}
	if info.Root == "" {
		t.Error("Root should not be empty")
	}
}

func TestDetector_RepoWithoutRemote(t *testing.T) {
	t.Parallel()
	tmp := initRepo(t, "")
	d := gitrepo.New()

	info, err := d.Detect(context.Background(), tmp)
	if !errors.Is(err, ports.ErrRepoHasNoRemote) {
		t.Errorf("got %v, want ErrRepoHasNoRemote", err)
	}
	// Root is still surfaced — callers may want the boundary even when no
	// canonical URL can be derived (e.g. to pick the notebook gracefully).
	if info.Root == "" {
		t.Error("Root should be surfaced even on no-remote error")
	}
	if info.URL != "" {
		t.Errorf("URL should be empty on no-remote error, got %q", info.URL)
	}
}

func TestDetector_FromSubdirectory(t *testing.T) {
	t.Parallel()
	// Pass a real remote so Detect returns success — the point of this test
	// is verifying that rev-parse climbs from a subdir to the repo root.
	tmp := initRepo(t, "git@github.com:Foo/Bar.git")
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	d := gitrepo.New()

	info, err := d.Detect(context.Background(), sub)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	// Root should resolve to the repo root, not the subdir we passed in.
	// Compare via real path because macOS /var → /private/var symlinks
	// would otherwise cause a spurious mismatch.
	wantReal, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("EvalSymlinks(tmp): %v", err)
	}
	gotReal, err := filepath.EvalSymlinks(info.Root)
	if err != nil {
		t.Fatalf("EvalSymlinks(info.Root): %v", err)
	}
	if gotReal != wantReal {
		t.Errorf("Root got %q, want %q", info.Root, tmp)
	}
}

func initRepo(t *testing.T, remoteURL string) string {
	t.Helper()
	tmp := t.TempDir()
	mustRun(t, tmp, "git", "init", "-q")
	mustRun(t, tmp, "git", "config", "user.email", "test@example.com")
	mustRun(t, tmp, "git", "config", "user.name", "test")
	if remoteURL != "" {
		mustRun(t, tmp, "git", "remote", "add", "origin", remoteURL)
	}
	return tmp
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}
