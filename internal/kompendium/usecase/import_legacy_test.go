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

func TestImportLegacy_MigratesDailyAndProject(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	legacy := &testutil.FakeLegacySource{
		Dailies: []ports.LegacyDaily{
			{Path: "/n/2026-04-22.md", Date: "2026-04-22", Body: []byte("first daily\n")},
			{Path: "/n/2026-04-25.md", Date: "2026-04-25", Body: []byte("today\n")},
		},
		Projects: []ports.LegacyProject{
			{Path: "/pn/dotfiles-008be364.md", URL: "git@github.com:serverkraken/dotfiles.git", Body: []byte("dotfiles body\n")},
		},
	}

	u := usecase.NewImportLegacy(store, legacy)
	out, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Migrated) != 3 {
		t.Errorf("Migrated got %+v, want 3 entries", out.Migrated)
	}
	if len(out.Skipped) != 0 {
		t.Errorf("Skipped got %+v, want none", out.Skipped)
	}

	// Daily landed at the expected ID with daily-typed frontmatter.
	got, err := store.Get(context.Background(), domain.ID("daily/2026-04-25"))
	if err != nil {
		t.Fatalf("Get daily: %v", err)
	}
	if got.Meta.Type != domain.TypeDaily || got.Meta.Date != "2026-04-25" {
		t.Errorf("daily meta got %+v", got.Meta)
	}

	// Project landed at the canonical-URL path with project meta.
	pid := domain.ID("projects/github.com/serverkraken/dotfiles/_project")
	pgot, err := store.Get(context.Background(), pid)
	if err != nil {
		t.Fatalf("Get project: %v", err)
	}
	if pgot.Meta.Project != "github.com/serverkraken/dotfiles" {
		t.Errorf("project canonical URL got %q", pgot.Meta.Project)
	}
}

func TestImportLegacy_SkipsExisting(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("daily/2026-04-25")
	pre, _ := domain.NewNote(id, domain.Frontmatter{
		ID: id.String(), Type: domain.TypeDaily, Date: "2026-04-25", Title: "preexisting",
	}, []byte("hand-written\n"))
	store.Seed(pre, time.Unix(1, 0))

	legacy := &testutil.FakeLegacySource{
		Dailies: []ports.LegacyDaily{
			{Path: "/n/2026-04-25.md", Date: "2026-04-25", Body: []byte("from legacy\n")},
		},
	}

	u := usecase.NewImportLegacy(store, legacy)
	out, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Migrated) != 0 {
		t.Errorf("nothing should migrate when target exists, got %+v", out.Migrated)
	}
	if len(out.Skipped) != 1 || out.Skipped[0].Reason == "" {
		t.Errorf("expected one skip with reason, got %+v", out.Skipped)
	}

	got, _ := store.Get(context.Background(), id)
	if got.Meta.Title != "preexisting" {
		t.Errorf("existing note overwritten, Title=%q", got.Meta.Title)
	}
}

func TestImportLegacy_SkipsProjectWithoutURL(t *testing.T) {
	t.Parallel()
	legacy := &testutil.FakeLegacySource{
		Projects: []ports.LegacyProject{
			{Path: "/pn/no-remote.md", URL: "", Body: []byte("body\n")},
		},
	}
	u := usecase.NewImportLegacy(testutil.NewFakeNoteStore(), legacy)
	out, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Migrated) != 0 || len(out.Skipped) != 1 {
		t.Errorf("got migrated=%+v skipped=%+v", out.Migrated, out.Skipped)
	}
}

func TestImportLegacy_DailyListError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced daily list err")
	u := usecase.NewImportLegacy(testutil.NewFakeNoteStore(), &testutil.FakeLegacySource{DailyErr: forced})
	_, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestImportLegacy_ProjectListError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced project list err")
	u := usecase.NewImportLegacy(testutil.NewFakeNoteStore(), &testutil.FakeLegacySource{ProjectErr: forced})
	_, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestImportLegacy_StoreErrorOnExists(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced exists err")
	store := testutil.NewFakeNoteStore()
	store.ExistsErr = forced
	legacy := &testutil.FakeLegacySource{
		Dailies: []ports.LegacyDaily{{Path: "/n/2026-04-25.md", Date: "2026-04-25", Body: []byte("x")}},
	}
	u := usecase.NewImportLegacy(store, legacy)
	_, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestImportLegacy_StoreErrorOnPut(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced put err")
	store := testutil.NewFakeNoteStore()
	store.PutErr = forced
	legacy := &testutil.FakeLegacySource{
		Dailies: []ports.LegacyDaily{{Path: "/n/2026-04-25.md", Date: "2026-04-25", Body: []byte("x")}},
	}
	u := usecase.NewImportLegacy(store, legacy)
	_, err := u.Execute(context.Background(), usecase.ImportLegacyInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
