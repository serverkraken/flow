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

func TestCreateDaily_NewlyCreated(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)}
	editor := &testutil.FakeEditor{}

	u := usecase.NewCreateDaily(store, clock, editor)
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !out.Created {
		t.Error("expected Created=true on first call")
	}
	if out.ID != "daily/2026-04-25" {
		t.Errorf("ID got %q, want daily/2026-04-25", out.ID)
	}
	if len(editor.Calls) != 1 {
		t.Errorf("editor should be called once, got %d calls: %+v", len(editor.Calls), editor.Calls)
	}

	// Verify the note was actually written.
	got, err := store.Get(context.Background(), out.ID)
	if err != nil {
		t.Fatalf("Get back: %v", err)
	}
	if got.Meta.Type != domain.TypeDaily {
		t.Errorf("Type got %q", got.Meta.Type)
	}
	if got.Meta.Date != "2026-04-25" {
		t.Errorf("Date got %q", got.Meta.Date)
	}
}

func TestCreateDaily_ReusesExisting(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	existing, _ := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25", Title: "preexisting"},
		[]byte("# already here\n"),
	)
	store.Seed(existing, time.Unix(1, 0))

	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)}
	editor := &testutil.FakeEditor{}

	u := usecase.NewCreateDaily(store, clock, editor)
	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if out.Created {
		t.Error("expected Created=false when note already exists")
	}
	got, err := store.Get(context.Background(), out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Title != "preexisting" {
		t.Errorf("existing note must not be overwritten, Title=%q", got.Meta.Title)
	}
}

func TestCreateDaily_ExistsError(t *testing.T) {
	t.Parallel()
	forced := errors.New("exists boom")
	store := testutil.NewFakeNoteStore()
	store.ExistsErr = forced

	u := usecase.NewCreateDaily(store, testutil.FixedClock{}, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped %v", err, forced)
	}
}

func TestCreateDaily_PutError(t *testing.T) {
	t.Parallel()
	forced := errors.New("put boom")
	store := testutil.NewFakeNoteStore()
	store.PutErr = forced

	u := usecase.NewCreateDaily(store, testutil.FixedClock{Time: time.Now().UTC()}, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped %v", err, forced)
	}
}

func TestCreateDaily_EditorError(t *testing.T) {
	t.Parallel()
	forced := errors.New("editor crashed")
	editor := &testutil.FakeEditor{Err: forced}

	u := usecase.NewCreateDaily(testutil.NewFakeNoteStore(), testutil.FixedClock{Time: time.Now().UTC()}, editor)
	_, err := u.Execute(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped %v", err, forced)
	}
}
