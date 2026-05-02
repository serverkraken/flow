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
	idx := testutil.NewFakeIndexer()

	id := domain.ID("daily/2026-04-25")
	store.Seed(mustNoteFull(id, domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))

	u := usecase.NewDeleteNote(store, idx)
	if err := u.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exists, _ := store.Exists(context.Background(), id); exists {
		t.Error("note should be removed from store")
	}
}

func TestDeleteNote_NotFound(t *testing.T) {
	t.Parallel()
	u := usecase.NewDeleteNote(testutil.NewFakeNoteStore(), nil)
	err := u.Execute(context.Background(), domain.ID("missing/note"))
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestDeleteNote_IndexErrorSwallowed(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("daily/2026-04-25")
	store.Seed(mustNoteFull(id, domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))

	idx := testutil.NewFakeIndexer()
	idx.DeleteErr = errors.New("forced index delete err")

	u := usecase.NewDeleteNote(store, idx)
	if err := u.Execute(context.Background(), id); err != nil {
		t.Errorf("index error must not surface as use-case error, got %v", err)
	}
	if exists, _ := store.Exists(context.Background(), id); exists {
		t.Error("store-side delete must still happen even if index fails")
	}
}

func TestDeleteNote_NilIndexIsOK(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("daily/2026-04-25")
	store.Seed(mustNoteFull(id, domain.TypeDaily, "", "2026-04-25"), time.Unix(1, 0))

	u := usecase.NewDeleteNote(store, nil)
	if err := u.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}
