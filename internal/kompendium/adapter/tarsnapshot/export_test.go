package tarsnapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/tarsnapshot"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestExport_SkipsGitDir(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "daily/2026-04-25.md", "x")
	writeFile(t, src, ".git/objects/abc", "git internals")
	writeFile(t, src, ".git/HEAD", "ref")

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := t.TempDir()
	if err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictAbort); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git", "objects", "abc")); err == nil {
		t.Error(".git/ should have been excluded from the archive")
	}
}

func TestExport_NonexistentSource(t *testing.T) {
	t.Parallel()
	err := tarsnapshot.New().Export(context.Background(),
		"/this-source-does-not-exist-xyz",
		filepath.Join(t.TempDir(), "out.tar.gz"))
	if err == nil {
		t.Error("expected error when source does not exist")
	}
}

func TestExport_BadOutPath(t *testing.T) {
	t.Parallel()
	err := tarsnapshot.New().Export(context.Background(), t.TempDir(), "/this-dir-does-not-exist/out.tar.gz")
	if err == nil {
		t.Error("expected error for unwritable out path")
	}
}

func TestExport_UnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	src := t.TempDir()
	writeFile(t, src, "x.md", "body")
	bad := filepath.Join(src, "x.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod 0o000 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	err := tarsnapshot.New().Export(context.Background(), src, filepath.Join(t.TempDir(), "out.tar.gz"))
	if err == nil {
		t.Error("expected read error during export")
	}
}
