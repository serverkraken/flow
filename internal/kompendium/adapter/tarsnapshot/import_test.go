package tarsnapshot_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/adapter/tarsnapshot"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestImport_ConflictAbort(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "daily/x.md", "from archive\n")

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "daily/x.md", "preexisting\n")

	err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictAbort)
	if err == nil {
		t.Error("expected conflict error in abort mode")
	}
}

func TestImport_ConflictNewer(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "x.md", "from archive\n")
	// Stamp the source file with a future mtime so it wins.
	future := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(filepath.Join(src, "x.md"), future, future); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "x.md", "preexisting\n")
	if err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictNewer); err != nil {
		t.Fatalf("Import newer: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "x.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from archive\n" {
		t.Errorf("newer mode should pick archive content, got %q", got)
	}
}

func TestImport_ConflictNewer_KeepsExistingWhenOlder(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "x.md", "older from archive\n")
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(src, "x.md"), past, past); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "x.md", "newer existing\n")
	if err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictNewer); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dst, "x.md"))
	if string(got) != "newer existing\n" {
		t.Errorf("newer mode should keep existing when archive is older, got %q", got)
	}
}

func TestImport_ConflictManual(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "x.md", "from archive\n")

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "x.md", "preexisting\n")
	if err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictManual); err != nil {
		t.Fatal(err)
	}

	existing, _ := os.ReadFile(filepath.Join(dst, "x.md"))
	imported, _ := os.ReadFile(filepath.Join(dst, "x.md.imported"))
	if string(existing) != "preexisting\n" {
		t.Errorf("existing must remain untouched in manual mode, got %q", existing)
	}
	if string(imported) != "from archive\n" {
		t.Errorf(".imported sidecar got %q", imported)
	}
}

func TestImport_UnknownConflictMode(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "x.md", "x")
	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	writeFile(t, dst, "x.md", "y")
	err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictMode(99))
	if err == nil {
		t.Error("expected error for unknown conflict mode")
	}
}

func TestImport_NonexistentArchive(t *testing.T) {
	t.Parallel()
	err := tarsnapshot.New().Import(context.Background(), "/no-such-archive.tar.gz", t.TempDir(), ports.ConflictAbort)
	if err == nil {
		t.Error("expected error for missing archive")
	}
}

func TestImport_NotGzip(t *testing.T) {
	t.Parallel()
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(bad, []byte("not actually gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := tarsnapshot.New().Import(context.Background(), bad, t.TempDir(), ports.ConflictAbort)
	if err == nil {
		t.Error("expected gzip error")
	}
}

func TestImport_RejectsNonLocalPath(t *testing.T) {
	t.Parallel()
	// Craft a tar archive whose header escapes the target root.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "../escape.md", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "evil.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	err := tarsnapshot.New().Import(context.Background(), archive, t.TempDir(), ports.ConflictAbort)
	if err == nil {
		t.Error("expected error for non-local archive path")
	}
}

// TestImport_RejectsOversizedEntry guards against decompression-bomb
// archives. An entry whose header claims a size beyond the per-entry
// cap must be rejected before any allocation or write to disk.
func TestImport_RejectsOversizedEntry(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "bomb.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	// Claim 1 GiB. No actual content needed — the header check fires
	// before extractEntry touches the stream.
	hdr := &tar.Header{
		Name:     "huge.md",
		Mode:     0o644,
		Size:     1 << 30,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	// We don't actually write 1 GiB — close the writer; the gzip stream
	// will be truncated, but tar.Next on the read side will still see
	// the header before EOF.
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()

	dst := t.TempDir()
	err = tarsnapshot.New().Import(context.Background(), archive, dst, ports.ConflictAbort)
	if err == nil {
		t.Fatal("expected size-cap error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceeds size cap")) {
		t.Errorf("error should mention size cap, got %q", err)
	}
}

func TestImport_TargetWriteFails(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o500 does not block writes")
	}
	src := t.TempDir()
	writeFile(t, src, "x.md", "body")
	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := os.Chmod(dst, 0o500); err != nil {
		t.Skipf("chmod 0o500 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dst, 0o755) })

	err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictAbort)
	if err == nil {
		t.Error("expected write error in read-only target dir")
	}
}
