package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

// Tries to force error branches in WriteFile / Append that the
// happy-path tests don't reach. They rely on filesystem behaviour
// (running as a non-root user, write-protected dir, etc.) so the
// assertions are loose: an error of any shape is acceptable, because
// the goal is to exercise the close-then-cleanup branches.

func TestWriteFile_ChmodErrorOnUnwritablePath(t *testing.T) {
	// Drop the parent dir's write perm AFTER WriteFile created the temp
	// file — then the os.Rename step will fail. Skip when running as root.
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod cannot block rename")
	}
	dir := t.TempDir()
	// Write a file once so the dir exists.
	target := filepath.Join(dir, "f.txt")
	if err := WriteFile(target, []byte("first"), 0o644); err != nil {
		t.Fatalf("first WriteFile: %v", err)
	}
	// Lock down the parent — chmod 0o500 blocks new file creation (CreateTemp).
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	// Second WriteFile should fail at CreateTemp.
	if err := WriteFile(target, []byte("second"), 0o644); err == nil {
		t.Errorf("WriteFile on locked dir should fail")
	}
}

func TestAppend_PermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod cannot block writes")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "log.tsv")
	// Pre-create a read-only file so the second-open O_APPEND fails.
	if err := os.WriteFile(target, []byte("seed\n"), 0o400); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := Append(target, []byte("row\n"), 0o644); err == nil {
		t.Errorf("Append to read-only file should fail")
	}
}
