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

func TestManager_IsRepo_NotARepo(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()

	got, err := m.IsRepo(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("IsRepo: %v", err)
	}
	if got {
		t.Error("empty tempdir should not be a repo")
	}
}

func TestManager_Init(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	m := gitsnapshot.New()

	if err := m.Init(context.Background(), tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := m.IsRepo(context.Background(), tmp)
	if err != nil {
		t.Fatalf("IsRepo after Init: %v", err)
	}
	if !got {
		t.Error("Init should leave the directory as a git repo")
	}

	// HEAD must resolve to a real commit (the initial empty one).
	out, err := exec.Command("git", "-C", tmp, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Errorf("rev-parse HEAD failed after Init: %v: %s", err, out)
	}

	// Init must seed a default .gitignore so .DS_Store and editor swap
	// files don't slip into the next snapshot — and into sync.
	gitignore := filepath.Join(tmp, ".gitignore")
	body, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("init should write a .gitignore: %v", err)
	}
	for _, want := range []string{".DS_Store", "*.swp", ".kompendium-*.tmp"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("default .gitignore missing %q, got:\n%s", want, body)
		}
	}
}

func TestManager_Init_PreservesExistingGitignore(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	custom := []byte("# my custom rules\nfoo/\n")
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	m := gitsnapshot.New()
	if err := m.Init(context.Background(), tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Errorf("Init must not overwrite an existing .gitignore.\ngot:  %s\nwant: %s", got, custom)
	}
}

func TestManager_HasUncommittedChanges(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	m := gitsnapshot.New()
	mustInit(t, m, tmp)

	dirty, err := m.HasUncommittedChanges(context.Background(), tmp)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if dirty {
		t.Error("freshly init'd repo should be clean")
	}

	if err := os.WriteFile(filepath.Join(tmp, "new.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirty, err = m.HasUncommittedChanges(context.Background(), tmp)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if !dirty {
		t.Error("dirty tree must be reported as uncommitted")
	}
}

func TestManager_Snapshot(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	m := gitsnapshot.New()
	mustInit(t, m, tmp)

	if err := os.WriteFile(filepath.Join(tmp, "note.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(context.Background(), tmp, "test snapshot"); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Last commit message must be ours.
	out, err := exec.Command("git", "-C", tmp, "log", "-1", "--format=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "test snapshot") {
		t.Errorf("last commit message got %q", out)
	}

	// And the tree should be clean again.
	dirty, _ := m.HasUncommittedChanges(context.Background(), tmp)
	if dirty {
		t.Error("Snapshot should leave the tree clean")
	}
}

func TestManager_Snapshot_RefusesEmpty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	m := gitsnapshot.New()
	mustInit(t, m, tmp)

	// No changes at all — Snapshot now surfaces git's "nothing to
	// commit" error. Callers (SnapshotNotebook) check
	// HasUncommittedChanges first and short-circuit on clean trees, so
	// reaching here without a real change is treated as a programming
	// error rather than silently producing an empty commit.
	if err := m.Snapshot(context.Background(), tmp, "empty snapshot"); err == nil {
		t.Error("Snapshot on clean tree should fail (git: nothing to commit)")
	}
}

func TestManager_Init_NonexistentDir(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()
	err := m.Init(context.Background(), "/this-directory-must-not-exist-xyz")
	if err == nil {
		t.Error("Init on a missing dir should fail")
	}
}

func mustInit(t *testing.T, m gitsnapshot.Manager, dir string) {
	t.Helper()
	if err := m.Init(context.Background(), dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
}
