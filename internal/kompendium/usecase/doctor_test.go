package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestDoctor_CleanNotebook(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{IsRepoValue: true}

	store.Seed(mustNoteFull("daily/2026-04-25", domain.TypeDaily, "", "2026-04-25"), tm(1))
	store.Seed(mustNoteFull("notes/setup", domain.TypeFree, "", ""), tm(2))

	u := usecase.NewDoctor(store, git)
	report, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !report.IsRepo {
		t.Error("expected IsRepo=true")
	}
	if report.HasUncommitted {
		t.Error("expected clean tree")
	}
	if report.NoteCount != 2 {
		t.Errorf("NoteCount got %d, want 2", report.NoteCount)
	}
	if !report.IsClean() {
		t.Errorf("expected IsClean()=true, report=%+v", report)
	}
}

func TestDoctor_NotARepo(t *testing.T) {
	t.Parallel()
	u := usecase.NewDoctor(testutil.NewFakeNoteStore(), &testutil.FakeNotebookInit{})

	report, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.IsRepo {
		t.Error("expected IsRepo=false")
	}
	if report.HasUncommitted {
		t.Error("HasUncommitted must be false on a non-repo")
	}
}

// TestDoctor_DetectsMergeMarkers covers the post-bundle-merge case:
// after `kompendium import --bundle` runs into a real conflict, the
// affected note carries `<<<<<<<` markers until the user resolves
// them. Doctor should surface this so you don't silently snapshot
// broken notes.
func TestDoctor_DetectsMergeMarkers(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{IsRepoValue: true}

	body := []byte("# Today\n\nSome text\n<<<<<<< HEAD\nlocal version\n=======\nremote version\n>>>>>>> kompendium-bundle/main\n")
	conflicted, err := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25"},
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	store.Seed(conflicted, tm(1))

	u := usecase.NewDoctor(store, git)
	report, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(report.MergeMarkers) != 1 {
		t.Fatalf("expected 1 merge-marker issue, got %+v", report.MergeMarkers)
	}
	if report.MergeMarkers[0].NoteID != "daily/2026-04-25" {
		t.Errorf("wrong note ID: %+v", report.MergeMarkers[0])
	}
	if report.IsClean() {
		t.Error("notebook with merge markers must not be reported as clean")
	}
}

func TestDoctor_DetectsBrokenLinks(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{IsRepoValue: true}

	src := mustNoteWithBody("daily/2026-04-25", domain.TypeDaily, "",
		"see [[notes/missing]] and [[notes/setup]] for context")
	target := mustNoteFull("notes/setup", domain.TypeFree, "", "")
	store.Seed(src, tm(1))
	store.Seed(target, tm(2))

	report, err := usecase.NewDoctor(store, git).Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.BrokenLinks) != 1 {
		t.Fatalf("BrokenLinks got %+v, want 1", report.BrokenLinks)
	}
	if report.BrokenLinks[0].NoteID != "daily/2026-04-25" {
		t.Errorf("NoteID got %q", report.BrokenLinks[0].NoteID)
	}
	if report.IsClean() {
		t.Error("broken link must mark report dirty")
	}
}

func TestDoctor_DetectsInvalidFrontmatter(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{}

	// Build a note that bypasses NewNote validation by writing the FM directly.
	bad := domain.Note{
		ID:   "daily/x",
		Meta: domain.Frontmatter{ID: "daily/x", Type: "garbage"},
	}
	store.Seed(bad, tm(1))

	report, err := usecase.NewDoctor(store, git).Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.InvalidFrontmatter) != 1 {
		t.Errorf("InvalidFrontmatter got %+v", report.InvalidFrontmatter)
	}
}

func TestDoctor_DetectsInconsistentIDs(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	git := &testutil.FakeNotebookInit{}

	// Frontmatter id intentionally drifts from the note's path-derived ID.
	drift := domain.Note{
		ID:   "daily/2026-04-25",
		Meta: domain.Frontmatter{ID: "daily/2026-04-22", Type: domain.TypeDaily},
	}
	store.Seed(drift, tm(1))

	report, err := usecase.NewDoctor(store, git).Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.InconsistentIDs) != 1 {
		t.Errorf("InconsistentIDs got %+v", report.InconsistentIDs)
	}
}

func TestDoctor_PropagatesErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		setup   func(*testutil.FakeNoteStore, *testutil.FakeNotebookInit)
		wantErr error
	}{
		{
			name:    "is-repo error",
			setup:   func(_ *testutil.FakeNoteStore, g *testutil.FakeNotebookInit) { g.IsRepoErr = errIsRepo },
			wantErr: errIsRepo,
		},
		{
			name: "has-changes error",
			setup: func(_ *testutil.FakeNoteStore, g *testutil.FakeNotebookInit) {
				g.IsRepoValue = true
				g.HasChangesErr = errHasChanges
			},
			wantErr: errHasChanges,
		},
		{
			name:    "list error",
			setup:   func(s *testutil.FakeNoteStore, _ *testutil.FakeNotebookInit) { s.ListErr = errList },
			wantErr: errList,
		},
		{
			name: "get error not ErrNoteNotFound",
			setup: func(s *testutil.FakeNoteStore, _ *testutil.FakeNotebookInit) {
				s.Seed(mustNoteFull("daily/x", domain.TypeDaily, "", ""), tm(1))
				s.GetErr = errGet
			},
			wantErr: errGet,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := testutil.NewFakeNoteStore()
			git := &testutil.FakeNotebookInit{}
			tc.setup(store, git)

			_, err := usecase.NewDoctor(store, git).Execute(context.Background())
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want wrapped %v", err, tc.wantErr)
			}
		})
	}
}

var (
	errIsRepo     = errors.New("forced is-repo")
	errHasChanges = errors.New("forced has-changes")
	errList       = errors.New("forced list")
	errGet        = errors.New("forced get")
)

// mustNoteFull is provided by render_daily_test.go in the same _test package.
// mustNoteWithBody is provided by render_backlinks_test.go.
