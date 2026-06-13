package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// modifyingEditor rewrites the tempfile with the given content when Edit is
// called, simulating a user who changes the note body.
type modifyingEditor struct {
	content string
	Calls   []string
}

func (e *modifyingEditor) Edit(_ context.Context, path string) error {
	e.Calls = append(e.Calls, path)
	return os.WriteFile(path, []byte(e.content), 0o600)
}

func seedNote(t *testing.T, store *testutil.FakeNoteStore, id domain.ID, body string) domain.Note {
	t.Helper()
	n, err := domain.NewNote(id, domain.Frontmatter{
		ID:   id.String(),
		Type: domain.TypeFree,
	}, []byte(body))
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	store.Seed(n, time.Unix(1, 0))
	return n
}

// TestEditNote_NoChange verifies that if the user does not modify the tempfile,
// Put is never called and the tempfile is removed.
func TestEditNote_NoChange(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("free/unchanged")
	n := seedNote(t, store, id, "# hello\n")

	// FakeEditor records the path but leaves the file untouched.
	editor := &testutil.FakeEditor{}

	u := usecase.EditNote{Store: store, Editor: editor}
	if err := u.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Put must not have been called.
	got, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Body) != string(n.Body) {
		t.Errorf("body changed without editor modification: %q", got.Body)
	}
	if len(editor.Calls) != 1 {
		t.Errorf("expected 1 editor call, got %d", len(editor.Calls))
	}

	// Tempfile must be cleaned up.
	if _, statErr := os.Stat(editor.Calls[0]); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("tempfile %q was not removed", editor.Calls[0])
		_ = os.Remove(editor.Calls[0])
	}
}

// TestEditNote_Change verifies that when the editor modifies the tempfile,
// Put is called with the new body and the tempfile is removed.
func TestEditNote_Change(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("free/changed")
	seedNote(t, store, id, "# old body\n")

	const newBody = "# new body\n"
	editor := &modifyingEditor{
		content: "---\nid: free/changed\ntype: free\n---\n" + newBody,
	}

	u := usecase.EditNote{Store: store, Editor: editor}
	if err := u.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Body) != newBody {
		t.Errorf("body = %q, want %q", got.Body, newBody)
	}

	// Tempfile must be cleaned up on success.
	if len(editor.Calls) == 1 {
		if _, statErr := os.Stat(editor.Calls[0]); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("tempfile %q was not removed after successful Put", editor.Calls[0])
			_ = os.Remove(editor.Calls[0])
		}
	}
}

// TestEditNote_Conflict verifies that on ErrVersionConflict the error
// message contains the tempfile path so the user can recover the edit.
func TestEditNote_Conflict(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("free/conflict")
	seedNote(t, store, id, "# original\n")

	// Force Put to return a version conflict using the kompendium sentinel.
	store.PutErr = kompports.ErrVersionConflict

	editor := &modifyingEditor{
		content: "---\nid: free/conflict\ntype: free\n---\n# edited\n",
	}

	u := usecase.EditNote{Store: store, Editor: editor}
	err := u.Execute(context.Background(), id)
	if err == nil {
		t.Fatal("expected error on version conflict, got nil")
	}
	if !errors.Is(err, kompports.ErrVersionConflict) {
		t.Errorf("error does not wrap ErrVersionConflict: %v", err)
	}

	// Error message must contain the tempfile path.
	if len(editor.Calls) != 1 {
		t.Fatalf("expected 1 editor call, got %d", len(editor.Calls))
	}
	tmpPath := editor.Calls[0]
	if !strings.Contains(err.Error(), tmpPath) {
		t.Errorf("error %q does not contain tempfile path %q", err.Error(), tmpPath)
	}

	// Tempfile must NOT be removed on conflict so the user can recover.
	if _, statErr := os.Stat(tmpPath); errors.Is(statErr, os.ErrNotExist) {
		t.Error("tempfile was removed on conflict — user cannot recover the edit")
	} else {
		_ = os.Remove(tmpPath)
	}
}
