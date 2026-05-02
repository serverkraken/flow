package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestOpen_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("daily/2026-04-25")
	pre, _ := domain.NewNote(id, domain.Frontmatter{
		ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25",
	}, []byte{})
	store.Seed(pre, time.Unix(1, 0))

	editor := &testutil.FakeEditor{}
	u := usecase.NewOpen(store, editor)

	if err := u.Execute(context.Background(), usecase.OpenInput{ID: id}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantPath := store.Path(id)
	if len(editor.Calls) != 1 || editor.Calls[0] != wantPath {
		t.Errorf("editor calls = %+v, want one call to %q", editor.Calls, wantPath)
	}
}

func TestOpen_NotFound(t *testing.T) {
	t.Parallel()
	u := usecase.NewOpen(testutil.NewFakeNoteStore(), &testutil.FakeEditor{})
	err := u.Execute(context.Background(), usecase.OpenInput{ID: "missing/note"})
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestOpen_ExistsError(t *testing.T) {
	t.Parallel()
	forced := errors.New("exists boom")
	store := testutil.NewFakeNoteStore()
	store.ExistsErr = forced

	u := usecase.NewOpen(store, &testutil.FakeEditor{})
	err := u.Execute(context.Background(), usecase.OpenInput{ID: "x/y"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestOpen_EditorError(t *testing.T) {
	t.Parallel()
	forced := errors.New("editor down")
	store := testutil.NewFakeNoteStore()
	id := domain.ID("daily/x")
	pre, _ := domain.NewNote(id, domain.Frontmatter{ID: "daily/x", Type: domain.TypeDaily}, []byte{})
	store.Seed(pre, time.Unix(1, 0))

	u := usecase.NewOpen(store, &testutil.FakeEditor{Err: forced})
	err := u.Execute(context.Background(), usecase.OpenInput{ID: id})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
