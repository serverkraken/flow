package gitworktree_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/gitworktree"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRootAndList(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-b", "main")
	git(t, main, "commit", "--allow-empty", "-m", "init")

	// Root from the main worktree.
	root, ok, err := gitworktree.Root(main)
	if err != nil || !ok {
		t.Fatalf("Root ok=%v err=%v", ok, err)
	}
	// macOS /var → /private/var symlink: compare resolved suffixes loosely.
	if !strings.HasSuffix(root, filepath.Base(main)) {
		t.Errorf("Root = %q, want suffix %q", root, filepath.Base(main))
	}

	// Add a linked worktree on a new branch.
	wt := main + "-wt"
	git(t, main, "worktree", "add", "-b", "feature", wt)

	wts, err := gitworktree.List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("want 2 worktrees, got %d: %+v", len(wts), wts)
	}
	var mainSeen, featSeen bool
	for _, w := range wts {
		if w.HeadShort == "" {
			t.Errorf("worktree %q missing HeadShort", w.Path)
		}
		if w.IsMain {
			mainSeen = true
		}
		if w.Branch == "feature" {
			featSeen = true
		}
	}
	if !mainSeen {
		t.Error("no worktree marked IsMain")
	}
	if !featSeen {
		t.Error("feature branch worktree not found")
	}
}

func TestRootNotAGitRepo(t *testing.T) {
	_, ok, err := gitworktree.Root(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Error("non-repo dir must report ok=false")
	}
}

func TestDirtyFlag(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-b", "main")
	git(t, main, "commit", "--allow-empty", "-m", "init")
	// create an untracked-but-ignored-free change → tracked file modification
	if err := exec.Command("sh", "-c", "echo hi > "+filepath.Join(main, "f.txt")).Run(); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", "f.txt")
	git(t, main, "commit", "-m", "add f")
	if err := exec.Command("sh", "-c", "echo changed > "+filepath.Join(main, "f.txt")).Run(); err != nil {
		t.Fatal(err)
	}
	root, _, _ := gitworktree.Root(main)
	wts, _ := gitworktree.List(root)
	if len(wts) != 1 || !wts[0].Dirty {
		t.Errorf("expected single dirty worktree, got %+v", wts)
	}
}
