package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestRenderDaily_AggregatesProjectsForSameDate(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(daily("daily/2026-04-25", "2026-04-25"), tm(10))
	store.Seed(projectDated("projects/foo/2026-04-25", "github.com/foo", "2026-04-25"), tm(11))
	store.Seed(projectDated("projects/bar/2026-04-25", "github.com/bar", "2026-04-25"), tm(12))
	store.Seed(projectDated("projects/foo/2026-04-22", "github.com/foo", "2026-04-22"), tm(9))

	u := usecase.NewRenderDaily(store)
	out, err := u.Execute(context.Background(), usecase.RenderDailyInput{DailyID: "daily/2026-04-25"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Daily.ID != "daily/2026-04-25" {
		t.Errorf("Daily got %q", out.Daily.ID)
	}
	wantOrder := []domain.ID{
		// Sort by project name first.
		"projects/bar/2026-04-25",
		"projects/foo/2026-04-25",
	}
	if len(out.ProjectsForDay) != len(wantOrder) {
		t.Fatalf("ProjectsForDay len = %d, want %d", len(out.ProjectsForDay), len(wantOrder))
	}
	for i, want := range wantOrder {
		if out.ProjectsForDay[i].ID != want {
			t.Errorf("pos %d: got %q, want %q", i, out.ProjectsForDay[i].ID, want)
		}
	}
}

func TestRenderDaily_DailyNotFound(t *testing.T) {
	t.Parallel()
	u := usecase.NewRenderDaily(testutil.NewFakeNoteStore())
	_, err := u.Execute(context.Background(), usecase.RenderDailyInput{DailyID: "daily/missing"})
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestRenderDaily_DailyHasNoDate(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	store.Seed(daily("daily/x", ""), tm(1))
	store.Seed(projectDated("projects/foo/2026-04-25", "github.com/foo", "2026-04-25"), tm(2))

	u := usecase.NewRenderDaily(store)
	out, err := u.Execute(context.Background(), usecase.RenderDailyInput{DailyID: "daily/x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ProjectsForDay) != 0 {
		t.Errorf("daily without Date must yield empty ProjectsForDay, got %d", len(out.ProjectsForDay))
	}
}

func TestRenderDaily_ListError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced list error")
	store := testutil.NewFakeNoteStore()
	store.Seed(daily("daily/2026-04-25", "2026-04-25"), tm(1))
	store.ListErr = forced

	u := usecase.NewRenderDaily(store)
	_, err := u.Execute(context.Background(), usecase.RenderDailyInput{DailyID: "daily/2026-04-25"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want %v", err, forced)
	}
}

// --- helpers ----------------------------------------------------------------

func daily(id, date string) domain.Note {
	return mustNoteFull(domain.ID(id), domain.TypeDaily, "", date)
}

func projectDated(id, project, date string) domain.Note {
	return mustNoteFull(domain.ID(id), domain.TypeProject, project, date)
}

func mustNoteFull(id domain.ID, typ domain.NoteType, project, date string) domain.Note {
	fm := domain.Frontmatter{
		ID:      id.String(),
		Type:    typ,
		Project: project,
		Date:    date,
		Title:   "title for " + id.String(),
	}
	n, err := domain.NewNote(id, fm, []byte("body of "+id.String()+"\n"))
	if err != nil {
		panic(err)
	}
	return n
}
