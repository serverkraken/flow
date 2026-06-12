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

func TestDeleteNote_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()

	id := domain.ID("daily/2026-04-25")
	store.Seed(mustNoteFull(id, domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))

	u := usecase.NewDeleteNote(store)
	if err := u.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exists, _ := store.Exists(context.Background(), id); exists {
		t.Error("note should be removed from store")
	}
}

func TestDeleteNote_NotFound(t *testing.T) {
	t.Parallel()
	u := usecase.NewDeleteNote(testutil.NewFakeNoteStore())
	err := u.Execute(context.Background(), domain.ID("missing/note"))
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}
