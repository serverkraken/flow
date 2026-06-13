package fssourcedirs_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/fssourcedirs"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.SourceDirScanner = (*fssourcedirs.Scanner)(nil)

func mkRepo(t *testing.T, root, rel string) {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// mkWorktree creates a worktree-style entry where .git is a *file*
// pointing at another path (rather than a directory).
func mkWorktree(t *testing.T, root, rel string) {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"),
		[]byte("gitdir: ../main/.git/worktrees/foo"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList_MissingRoot(t *testing.T) {
	got, err := fssourcedirs.New(filepath.Join(t.TempDir(), "missing")).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestList_RootIsFile(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "file")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fssourcedirs.New(regular).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for non-dir root, got %v", got)
	}
}

func TestList_FlatRepos(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "alpha")
	mkRepo(t, root, "bravo")

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 projects, got %d: %v", len(got), got)
	}
	if got[0].Name != "alpha" || got[1].Name != "bravo" {
		t.Errorf("order: got %v", []string{got[0].Name, got[1].Name})
	}
	if got[0].Path != filepath.Join(root, "alpha") {
		t.Errorf("absolute path: got %q", got[0].Path)
	}
}

func TestList_NestedRepos(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "owner/repo-a")
	mkRepo(t, root, "owner/repo-b")
	mkRepo(t, root, "standalone")

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"owner/repo-a", "owner/repo-b", "standalone"}
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names: got %v, want %v", names, want)
	}
}

func TestList_GitFileWorktree(t *testing.T) {
	root := t.TempDir()
	mkWorktree(t, root, "wt")

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "wt" {
		t.Errorf("worktree: got %v", got)
	}
}

func TestList_DoesNotDescendIntoGitDir(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "outer")
	// Create a .git/inner/.git accidental nested entry that should NOT
	// register as a project.
	deep := filepath.Join(root, "outer", ".git", "modules", "inner", ".git")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "outer" {
		t.Errorf("got %v — should not include entries inside .git", got)
	}
}

func TestList_RespectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	// Repo deeper than MaxDepth must be ignored.
	deep := "a/b/c/d/e/f/g" // 7 levels, MaxDepth = 5 → ignored
	mkRepo(t, root, deep)
	mkRepo(t, root, "shallow")

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name
	}
	if !reflect.DeepEqual(names, []string{"shallow"}) {
		t.Errorf("got %v — depth-7 repo should be skipped", names)
	}
}

func TestList_TolerantToUnreadableSubtree(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 doesn't trigger EACCES")
	}
	root := t.TempDir()
	mkRepo(t, root, "readable")

	blocked := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(blocked, 0o755) // restore so TempDir cleanup works
	})

	got, err := fssourcedirs.New(root).List()
	if err != nil {
		t.Fatalf("want tolerant scan, got %v", err)
	}
	if len(got) != 1 || got[0].Name != "readable" {
		t.Errorf("readable repo missing under partial-failure scan: %v", got)
	}
}

func TestList_PathHandling_TrailingSlashRoot(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "proj")

	// New should normalise the root via Clean — trailing slash must
	// not double up in subsequent path arithmetic.
	s := fssourcedirs.New(root + string(filepath.Separator))
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "proj" {
		t.Errorf("got %v", got)
	}
	if got[0].Path != filepath.Join(root, "proj") {
		t.Errorf("absolute path: got %q", got[0].Path)
	}
}
