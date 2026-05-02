package gitsnapshot_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/gitsnapshot"
)

func TestBundle_ExportImportRoundtrip(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()
	ctx := context.Background()

	// Source repo with a committed file.
	src := t.TempDir()
	if err := m.Init(ctx, src); err != nil {
		t.Fatalf("Init src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "src.md"), []byte("source body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, src, "src commit"); err != nil {
		t.Fatalf("Snapshot src: %v", err)
	}

	bundlePath := filepath.Join(t.TempDir(), "snap.bundle")
	if err := m.ExportBundle(ctx, src, bundlePath); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	// Destination repo, separately initialised.
	dst := t.TempDir()
	if err := m.Init(ctx, dst); err != nil {
		t.Fatalf("Init dst: %v", err)
	}

	if err := m.ImportBundle(ctx, dst, bundlePath); err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "src.md")); err != nil {
		t.Errorf("src.md should be present in dst after bundle import: %v", err)
	}
}

func TestBundle_ImportConflictBubblesUp(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()
	ctx := context.Background()

	src := t.TempDir()
	if err := m.Init(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "x.md"), []byte("source content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, src, "src x"); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(t.TempDir(), "snap.bundle")
	if err := m.ExportBundle(ctx, src, bundlePath); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := m.Init(ctx, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "x.md"), []byte("conflicting target content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Snapshot(ctx, dst, "dst x"); err != nil {
		t.Fatal(err)
	}

	err := m.ImportBundle(ctx, dst, bundlePath)
	if err == nil {
		t.Error("expected merge conflict error when both ends modify the same file")
	}
}

func TestBundle_VerifyRejectsBadArchive(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()

	dst := t.TempDir()
	if err := m.Init(context.Background(), dst); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(t.TempDir(), "bad.bundle")
	if err := os.WriteFile(bad, []byte("not a git bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := m.ImportBundle(context.Background(), dst, bad)
	if err == nil {
		t.Error("expected bundle verify to reject a non-bundle file")
	}
}

func TestBundle_ExportNonRepoFails(t *testing.T) {
	t.Parallel()
	m := gitsnapshot.New()
	err := m.ExportBundle(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "snap.bundle"))
	if err == nil {
		t.Error("export from a non-repo should fail")
	}
	// Sanity-check that we can still test git is available.
	if _, lookErr := exec.LookPath("git"); lookErr != nil {
		t.Skipf("git not on PATH: %v", lookErr)
	}
}
