package atomicfile

import (
	"os"
	"path/filepath"
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

func TestSyncDir_MissingDir(t *testing.T) {
	if err := SyncDir(filepath.Join(t.TempDir(), "no-such-dir")); err == nil {
		t.Error("expected error for missing dir")
	}
}
