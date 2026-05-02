package gitsnapshot_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/gitsnapshot"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestRemote_GetUnsetReturnsSentinel(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	m := gitsnapshot.New()
	ctx := context.Background()
	tmp := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, tmp); err != nil {
		t.Fatal(err)
	}

	_, err := m.GetRemote(ctx, tmp)
	if !errors.Is(err, ports.ErrNoRemoteConfigured) {
		t.Errorf("got %v, want ErrNoRemoteConfigured", err)
	}
}

func TestRemote_SetThenGet(t *testing.T) {
	m := gitsnapshot.New()
	ctx := context.Background()
	tmp := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, tmp); err != nil {
		t.Fatal(err)
	}

	url := "https://example.test/notes.git"
	if err := m.SetRemote(ctx, tmp, url); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	got, err := m.GetRemote(ctx, tmp)
	if err != nil {
		t.Fatalf("GetRemote: %v", err)
	}
	if got != url {
		t.Errorf("got %q, want %q", got, url)
	}

	// Re-setting must not error (we need set-url, not just add).
	url2 := "git@example.test:notes.git"
	if err := m.SetRemote(ctx, tmp, url2); err != nil {
		t.Fatalf("re-SetRemote: %v", err)
	}
	got, err = m.GetRemote(ctx, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if got != url2 {
		t.Errorf("got %q after re-set, want %q", got, url2)
	}
}

func TestRemote_SyncWithoutRemoteFails(t *testing.T) {
	m := gitsnapshot.New()
	ctx := context.Background()
	tmp := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, tmp); err != nil {
		t.Fatal(err)
	}

	_, err := m.Sync(ctx, tmp)
	if !errors.Is(err, ports.ErrNoRemoteConfigured) {
		t.Errorf("got %v, want ErrNoRemoteConfigured", err)
	}
}

// TestRemote_SyncRoundtrip points two notebooks at a shared bare repo
// (the "remote") and verifies that `kompendium sync` from each side
// converges them: a commit made on side A shows up on side B after a
// sync from B.
func TestRemote_SyncRoundtrip(t *testing.T) {
	m := gitsnapshot.New()
	ctx := context.Background()

	// Bare "remote" repo.
	bareDir := newRepoWithoutIdentity(t)
	if err := runGit(t, bareDir, "init", "--bare", "-q", "-b", "main"); err != nil {
		t.Fatal(err)
	}

	// Two clones pointing at it.
	a := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRemote(ctx, a, bareDir); err != nil {
		t.Fatal(err)
	}

	b := newRepoWithoutIdentity(t)
	if err := m.Init(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRemote(ctx, b, bareDir); err != nil {
		t.Fatal(err)
	}

	// A writes a note, snapshots, syncs. The commit must land on the bare repo.
	if err := os.WriteFile(filepath.Join(a, "n.md"), []byte("from A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, a, "from A"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Sync(ctx, a); err != nil {
		t.Fatalf("Sync from A: %v", err)
	}

	// B syncs and must now see A's note. Sync runs pull then push; B has
	// no commits to push beyond the initial empty tree, so the round-trip
	// is essentially a fast-forward pull.
	if _, err := m.Sync(ctx, b); err != nil {
		t.Fatalf("Sync from B: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b, "n.md")); err != nil {
		t.Errorf("A's commit should be visible on B after sync, got: %v", err)
	}

	// Sanity: the bare repo received both pushes (A explicitly, B as a
	// no-op since pull caught it up). `git -C bare log --oneline` should
	// show at least the "from A" commit message.
	log, err := runGitOutput(t, bareDir, "log", "--oneline")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "from A") {
		t.Errorf("bare repo missing A's commit: %q", log)
	}
}

// runGit is a thin convenience over exec.Command for shelling out to git
// with the test's scrubbed-identity environment.
func runGit(t *testing.T, dir string, args ...string) error {
	t.Helper()
	_, err := runGitOutput(t, dir, args...)
	return err
}

func runGitOutput(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	out := mustGitOutput(t, dir, args...)
	return out, nil
}
