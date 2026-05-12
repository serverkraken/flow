package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteFile_CreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if err := WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("first write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("first content: got %q, want %q", got, "hello")
	}

	if err := WriteFile(path, []byte("world!!!"), 0o644); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "world!!!" {
		t.Errorf("replaced content: got %q, want %q", got, "world!!!")
	}

	// No leftover tempfile.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tempfile not cleaned up: %v", err)
	}
}

func TestWriteFile_PreservesPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("perm: got %o, want 0600", st.Mode().Perm())
	}
}

func TestWriteFile_ParentMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-subdir", "f.txt")
	err := WriteFile(path, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected error when parent dir is missing")
	}
}

func TestAppend_CreatesAndAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.tsv")

	if err := Append(path, []byte("row1\n"), 0o644); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := Append(path, []byte("row2\n"), 0o644); err != nil {
		t.Fatalf("second append: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(got) != "row1\nrow2\n" {
		t.Errorf("content: got %q, want %q", got, "row1\nrow2\n")
	}
}

// TestWriteFile_ConcurrentWritersDoNotCollide guards review finding
// Q1: two concurrent WriteFile calls on the same path must each pick a
// unique temp slot. Pre-fix the temp filename was a fixed `<path>.tmp`
// suffix, so two writers raced on the same file (corrupt content + the
// second Rename pulling the rug out from under the first).
//
// We can't reliably observe the pre-fix race in a test (it's
// probabilistic), but we can guarantee the post-fix invariant: after
// many concurrent writers, the directory contains exactly the target
// file and no leftover .tmp companions.
func TestWriteFile_ConcurrentWritersDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.txt")

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			payload := []byte(strings.Repeat("x", 1024) + "\n")
			if err := WriteFile(path, payload, 0o644); err != nil {
				errs <- err
				_ = i
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent writer: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %q", e.Name())
		}
	}
	// Final file content must be exactly one writer's payload (1025
	// bytes of "x…\n"), not a concatenation or torn partial.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if len(got) != 1025 || got[len(got)-1] != '\n' {
		t.Errorf("torn write: len=%d, last=%q", len(got), got[len(got)-1])
	}
}

func TestSyncDir_MissingDir(t *testing.T) {
	if err := SyncDir(filepath.Join(t.TempDir(), "no-such-dir")); err == nil {
		t.Error("expected error for missing dir")
	}
}

// TestSyncDir_Success pins the happy path: a real directory fsyncs
// without error. The other tests exercise SyncDir transitively via
// WriteFile but never assert it directly — without this, regressing
// SyncDir to a no-op or silent-error would only fail the durability
// invariant after a crash.
func TestSyncDir_Success(t *testing.T) {
	if err := SyncDir(t.TempDir()); err != nil {
		t.Errorf("SyncDir on a real directory should succeed, got %v", err)
	}
}

// TestAppend_ParentMissing pins the early-error path: the O_CREATE
// probe at the top of Append fails when the parent directory does not
// exist, and the function must propagate that error before opening
// the file for append. Pre-fix this branch was uncovered (review
// finding TEST-11 — atomicfile.Append error paths).
func TestAppend_ParentMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-subdir", "log.tsv")
	if err := Append(path, []byte("x"), 0o644); err == nil {
		t.Fatal("expected error when parent dir is missing")
	}
}

// TestAppend_PathIsDirectory exercises the second OpenFile (O_APPEND)
// error path: when the target path is a directory, the probe fails
// with IsExist (so Append continues) but the append-mode reopen then
// fails with EISDIR. Without this test the error-return branch at the
// "OpenFile O_APPEND" call site is uncovered.
func TestAppend_PathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := Append(filepath.Join(dir, "x"), []byte("payload\n"), 0o644); err == nil {
		t.Fatal("expected error when path is a directory")
	}
}
