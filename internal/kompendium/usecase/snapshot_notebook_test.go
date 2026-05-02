package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestSnapshotNotebook_CleanTree(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeNotebookInit{} // HasChangesValue defaults to false

	u := usecase.NewSnapshotNotebook(testutil.NewFakeNoteStore(), git)
	out, err := u.Execute(context.Background(), usecase.SnapshotNotebookInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.HadChanges || out.Committed {
		t.Errorf("clean tree must produce no commit, got %+v", out)
	}
	if len(git.Snapshots) != 0 {
		t.Errorf("Snapshot should not be called, got %+v", git.Snapshots)
	}
}

func TestSnapshotNotebook_DirtyTreeUsesDefaultMessage(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeNotebookInit{HasChangesValue: true}

	u := usecase.NewSnapshotNotebook(testutil.NewFakeNoteStore(), git)
	out, err := u.Execute(context.Background(), usecase.SnapshotNotebookInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Committed {
		t.Error("dirty tree must commit")
	}
	if len(git.Snapshots) != 1 || git.Snapshots[0] != "kompendium snapshot" {
		t.Errorf("default message wrong: %+v", git.Snapshots)
	}
}

func TestSnapshotNotebook_DirtyTreeUsesCustomMessage(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeNotebookInit{HasChangesValue: true}

	u := usecase.NewSnapshotNotebook(testutil.NewFakeNoteStore(), git)
	_, err := u.Execute(context.Background(), usecase.SnapshotNotebookInput{Message: "custom snap"})
	if err != nil {
		t.Fatal(err)
	}
	if len(git.Snapshots) != 1 || git.Snapshots[0] != "custom snap" {
		t.Errorf("custom message lost: %+v", git.Snapshots)
	}
}

func TestSnapshotNotebook_HasChangesError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced has-changes err")
	git := &testutil.FakeNotebookInit{HasChangesErr: forced}

	u := usecase.NewSnapshotNotebook(testutil.NewFakeNoteStore(), git)
	_, err := u.Execute(context.Background(), usecase.SnapshotNotebookInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestSnapshotNotebook_SnapshotError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced snap err")
	git := &testutil.FakeNotebookInit{HasChangesValue: true, SnapshotErr: forced}

	u := usecase.NewSnapshotNotebook(testutil.NewFakeNoteStore(), git)
	_, err := u.Execute(context.Background(), usecase.SnapshotNotebookInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
