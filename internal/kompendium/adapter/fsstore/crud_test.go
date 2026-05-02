package fsstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestStore_PutGetRoundtrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")

	if err := s.Put(context.Background(), n); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID got %q, want %q", got.ID, n.ID)
	}
	if got.Meta.Type != n.Meta.Type {
		t.Errorf("Type got %q, want %q", got.Meta.Type, n.Meta.Type)
	}
	if string(got.Body) != string(n.Body) {
		t.Errorf("Body got %q, want %q", got.Body, n.Body)
	}
}

func TestStore_Put_CreatesNestedDirs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "projects/github.com/foo/bar/2026-04-25", domain.TypeProject, "github.com/foo/bar")

	if err := s.Put(context.Background(), n); err != nil {
		t.Fatalf("Put: %v", err)
	}
	expected := filepath.Join(s.Root(), "projects", "github.com", "foo", "bar", "2026-04-25.md")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("nested file not created: %v", err)
	}
}

func TestStore_Put_OverwritesExisting(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := domain.ID("daily/2026-04-25")
	first := makeNote(t, string(id), domain.TypeDaily, "")
	first.Body = []byte("first version\n")

	if err := s.Put(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	second := makeNote(t, string(id), domain.TypeDaily, "")
	second.Body = []byte("second version\n")
	if err := s.Put(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Body) != "second version\n" {
		t.Errorf("expected overwritten body, got %q", got.Body)
	}
}

func TestStore_Put_MkdirFails(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	// Plant a regular file where the next Put expects a directory.
	blocker := filepath.Join(s.Root(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := makeNote(t, "blocker/child", domain.TypeFree, "")
	err := s.Put(context.Background(), n)
	if err == nil {
		t.Error("expected error when parent path is a file")
	}
}

func TestStore_Put_WriteFails(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod does not block writes")
	}
	s := newStore(t)
	// Pre-create the directory the note will land in, then strip write
	// permission so the atomic-write path (CreateTemp inside the dir) fails.
	noteDir := filepath.Join(s.Root(), "daily")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(noteDir, 0o500); err != nil {
		t.Skipf("chmod 0o500 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(noteDir, 0o755) })

	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	err := s.Put(context.Background(), n)
	if err == nil {
		t.Error("expected write error when target directory is not writable")
	}
}

// TestStore_Put_AtomicReplace verifies that a previously read-only target
// is replaced cleanly — atomic rename does not depend on the target's
// mode, only on the parent directory's write permission. This is the
// behavior the sync story needs: a read-only quirk on one machine must
// not block a write on another.
func TestStore_Put_AtomicReplace(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "v1 body")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Root(), "daily", "2026-04-25.md")
	if err := os.Chmod(p, 0o444); err != nil {
		t.Skipf("chmod 0o444 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	n2 := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "v2 body")
	if err := s.Put(context.Background(), n2); err != nil {
		t.Fatalf("Put should succeed via atomic rename even if old file is 0o444: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "v2 body") {
		t.Errorf("file content not replaced; got %q", got)
	}
}

// TestStore_Put_NoLeftoverTmpOnSuccess ensures the temp file the atomic
// write goes through is gone after a successful rename — a leftover
// .kompendium-*.tmp would land in the next git snapshot otherwise.
func TestStore_Put_NoLeftoverTmpOnSuccess(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "body")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(s.Root(), "daily"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			t.Errorf("unexpected leftover file %q in notebook", e.Name())
		}
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.Get(context.Background(), domain.ID("missing/note"))
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestStore_Get_BadFrontmatter(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := filepath.Join(s.Root(), "broken.md")
	if err := os.WriteFile(p, []byte("no frontmatter at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Get(context.Background(), domain.ID("broken"))
	if err == nil {
		t.Error("expected parse error")
	}
	if errors.Is(err, ports.ErrNoteNotFound) {
		t.Error("bad frontmatter must not surface as ErrNoteNotFound")
	}
}

func TestStore_Get_ReadError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Root(), "daily", "2026-04-25.md")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Skipf("chmod 0o000 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	_, err := s.Get(context.Background(), n.ID)
	if err == nil {
		t.Error("expected read error on unreadable file")
	}
	if errors.Is(err, ports.ErrNoteNotFound) {
		t.Error("permission error must not surface as ErrNoteNotFound")
	}
}

func TestStore_Delete_OK(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, _ := s.Exists(context.Background(), n.ID)
	if exists {
		t.Error("note should be gone after Delete")
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	err := s.Delete(context.Background(), domain.ID("missing/note"))
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestStore_Delete_RemoveError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o500 does not prevent rm")
	}
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.Root(), "daily")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := s.Delete(context.Background(), n.ID)
	if err == nil {
		t.Error("expected remove error on read-only parent dir")
	}
	if errors.Is(err, ports.ErrNoteNotFound) {
		t.Error("permission error must not surface as ErrNoteNotFound")
	}
}

func TestStore_Exists(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	got, err := s.Exists(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !got {
		t.Error("expected true for existing note")
	}

	got, err = s.Exists(context.Background(), domain.ID("missing/x"))
	if err != nil {
		t.Fatalf("Exists missing: %v", err)
	}
	if got {
		t.Error("expected false for missing note")
	}
}

func TestStore_Exists_StatError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — directory permissions not enforced")
	}
	s := newStore(t)
	dir := filepath.Join(s.Root(), "blocked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := s.Exists(context.Background(), domain.ID("blocked/whatever"))
	if err == nil {
		t.Error("expected stat error for unreadable parent dir")
	}
}
