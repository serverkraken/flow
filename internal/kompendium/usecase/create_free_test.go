package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestCreateFree_NewlyCreated(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	editor := &testutil.FakeEditor{}

	u := usecase.NewCreateFree(store, editor)
	out, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "setup", Title: "Initial setup"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Created {
		t.Error("expected Created=true")
	}
	if out.ID != "notes/setup" {
		t.Errorf("ID got %q, want notes/setup", out.ID)
	}

	got, err := store.Get(context.Background(), out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Title != "Initial setup" || got.Meta.Type != domain.TypeFree {
		t.Errorf("frontmatter not set as expected: %+v", got.Meta)
	}
	if len(editor.Calls) != 1 {
		t.Errorf("editor calls = %d, want 1", len(editor.Calls))
	}
}

func TestCreateFree_ReusesExisting(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("notes/setup")
	pre, _ := domain.NewNote(id, domain.Frontmatter{
		ID: "notes/setup", Type: domain.TypeFree, Title: "preexisting",
	}, []byte("body\n"))
	store.Seed(pre, time.Unix(1, 0))

	u := usecase.NewCreateFree(store, &testutil.FakeEditor{})
	out, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "setup"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Created {
		t.Error("expected Created=false")
	}
	got, _ := store.Get(context.Background(), id)
	if got.Meta.Title != "preexisting" {
		t.Errorf("existing overwritten")
	}
}

func TestCreateFree_EmptySlug(t *testing.T) {
	t.Parallel()
	u := usecase.NewCreateFree(testutil.NewFakeNoteStore(), &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateFreeInput{})
	if !errors.Is(err, usecase.ErrSlugRequired) {
		t.Errorf("got %v, want ErrSlugRequired", err)
	}
}

func TestCreateFree_InvalidSlug(t *testing.T) {
	t.Parallel()
	u := usecase.NewCreateFree(testutil.NewFakeNoteStore(), &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "../escape"})
	if err == nil {
		t.Fatal("expected error for traversal slug")
	}
	if !errors.Is(err, domain.ErrInvalidID) {
		t.Errorf("got %v, want wrapped ErrInvalidID", err)
	}
}

func TestCreateFree_ExistsError(t *testing.T) {
	t.Parallel()
	forced := errors.New("exists boom")
	store := testutil.NewFakeNoteStore()
	store.ExistsErr = forced

	u := usecase.NewCreateFree(store, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestCreateFree_PutError(t *testing.T) {
	t.Parallel()
	forced := errors.New("put boom")
	store := testutil.NewFakeNoteStore()
	store.PutErr = forced

	u := usecase.NewCreateFree(store, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestCreateFree_EditorError(t *testing.T) {
	t.Parallel()
	forced := errors.New("editor down")
	editor := &testutil.FakeEditor{Err: forced}

	u := usecase.NewCreateFree(testutil.NewFakeNoteStore(), editor)
	_, err := u.Execute(context.Background(), usecase.CreateFreeInput{Slug: "x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
