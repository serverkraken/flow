package fsstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestNew_OK(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "notebook")

	s, err := fsstore.New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Root() != root {
		t.Errorf("Root() = %q, want %q", s.Root(), root)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("notebook root not created: %v", err)
	}
}

func TestNew_ParentIsFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := fsstore.New(filepath.Join(blocker, "child"))
	if err == nil {
		t.Error("expected error when parent path is a regular file")
	}
}

// --- helpers shared with crud_test.go / list_test.go ---------------------

func newStore(t *testing.T) *fsstore.Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "notebook")
	s, err := fsstore.New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func makeNote(t *testing.T, id string, typ domain.NoteType, project string) domain.Note {
	t.Helper()
	parsed, err := domain.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", id, err)
	}
	fm := domain.Frontmatter{
		ID:      parsed.String(),
		Type:    typ,
		Project: project,
		Title:   "test " + id,
	}
	n, err := domain.NewNote(parsed, fm, []byte("body of "+id+"\n"))
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}
