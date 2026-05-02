package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestInitNotebook_FreshDirectory(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{}

	u := usecase.NewInitNotebook(store, git)
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.AlreadyInitialized {
		t.Error("AlreadyInitialized must be false on fresh dir")
	}
	if !git.Initialized {
		t.Error("Init must have been called")
	}
	if out.Root != store.Root() {
		t.Errorf("Root got %q, want %q", out.Root, store.Root())
	}
}

func TestInitNotebook_Idempotent(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{IsRepoValue: true}

	u := usecase.NewInitNotebook(store, git)
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.AlreadyInitialized {
		t.Error("AlreadyInitialized must be true when IsRepoValue is true")
	}
	if git.Initialized {
		t.Error("Init must not be called when notebook is already a repo")
	}
}

func TestInitNotebook_IsRepoError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced is-repo err")
	git := &testutil.FakeNotebookInit{IsRepoErr: forced}

	u := usecase.NewInitNotebook(testutil.NewFakeNoteStore(), git)
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestInitNotebook_InitError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced init err")
	git := &testutil.FakeNotebookInit{InitErr: forced}

	u := usecase.NewInitNotebook(testutil.NewFakeNoteStore(), git)
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}
