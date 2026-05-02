package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
)

func TestNewDaily_Created(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "new", "daily")
	if err != nil {
		t.Fatalf("new daily: %v", err)
	}
	if !strings.Contains(stdout, "Created daily/2026-04-25") {
		t.Errorf("output got %q", stdout)
	}
	if len(env.editor.Calls) != 1 {
		t.Errorf("editor not called once: %+v", env.editor.Calls)
	}
}

func TestNewProject_FromCwdFlag(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.repo.Info = ports.RepoInfo{Root: "/repos/foo", URL: domain.CanonicalURL("github.com/foo/bar")}

	stdout, _, err := runCmd(t, env.deps, "new", "project", "--cwd", "/repos/foo")
	if err != nil {
		t.Fatalf("new project: %v", err)
	}
	if !strings.Contains(stdout, "projects/github.com/foo/bar/2026-04-25") {
		t.Errorf("output got %q", stdout)
	}
}

func TestNewProject_NotInRepo(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.repo.Err = ports.ErrNotInRepo

	_, _, err := runCmd(t, env.deps, "new", "project", "--cwd", "/nowhere")
	if !errors.Is(err, ports.ErrNotInRepo) {
		t.Errorf("got %v, want ErrNotInRepo", err)
	}
}

func TestNewFree_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "new", "free", "setup", "--title", "Initial setup")
	if err != nil {
		t.Fatalf("new free: %v", err)
	}
	if !strings.Contains(stdout, "Created notes/setup") {
		t.Errorf("output got %q", stdout)
	}
}

func TestNewFree_BadSlug(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "new", "free", "../escape")
	if err == nil {
		t.Fatal("expected error for traversal slug")
	}
}

func TestNewProject_OsGetwdFallback(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.repo.Err = ports.ErrNotInRepo
	// Without --cwd, the command falls back to os.Getwd(); the FakeRepoDetector
	// returns ErrNotInRepo regardless of the cwd value, which exercises the
	// os.Getwd path without needing a real repo.
	_, _, err := runCmd(t, env.deps, "new", "project")
	if !errors.Is(err, ports.ErrNotInRepo) {
		t.Errorf("got %v, want ErrNotInRepo", err)
	}
}

// shut up lint about unused testutil if no test references it directly.
var _ = testutil.NewFakeNoteStore
