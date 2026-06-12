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

func TestCreateProject_NewlyCreated(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	repo := &testutil.FakeRepoDetector{
		Info: ports.RepoInfo{
			Root: "/repos/dotfiles",
			URL:  domain.CanonicalURL("github.com/serverkraken/dotfiles"),
		},
	}
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)}
	editor := &testutil.FakeEditor{}

	u := usecase.NewCreateProject(store, repo, clock, editor)
	out, err := u.Execute(context.Background(), usecase.CreateProjectInput{Cwd: "/repos/dotfiles/sub"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Created {
		t.Error("expected Created=true on first call")
	}
	wantID := domain.ID("projects/github.com/serverkraken/dotfiles/2026-04-25")
	if out.ID != wantID {
		t.Errorf("ID got %q, want %q", out.ID, wantID)
	}
	if out.Project != "github.com/serverkraken/dotfiles" {
		t.Errorf("Project got %q", out.Project)
	}
	if len(editor.Calls) != 1 {
		t.Errorf("editor should be called once, got %d calls: %+v", len(editor.Calls), editor.Calls)
	}

	got, err := store.Get(context.Background(), out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Project != "github.com/serverkraken/dotfiles" {
		t.Errorf("Project frontmatter got %q", got.Meta.Project)
	}
	if got.Meta.Date != "2026-04-25" {
		t.Errorf("Date got %q, want 2026-04-25", got.Meta.Date)
	}
}

func TestCreateProject_NotInRepo(t *testing.T) {
	t.Parallel()
	repo := &testutil.FakeRepoDetector{Err: ports.ErrNotInRepo}
	u := usecase.NewCreateProject(testutil.NewFakeNoteStore(), repo, testutil.FixedClock{}, &testutil.FakeEditor{})

	_, err := u.Execute(context.Background(), usecase.CreateProjectInput{Cwd: "/nonrepo"})
	if !errors.Is(err, ports.ErrNotInRepo) {
		t.Errorf("got %v, want ErrNotInRepo", err)
	}
}

func TestCreateProject_ReusesExisting(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	id := domain.ID("projects/github.com/foo/bar/2026-04-25")
	pre, _ := domain.NewNote(id, domain.Frontmatter{
		ID:      id.String(),
		Type:    domain.TypeProject,
		Project: "github.com/foo/bar",
		Date:    "2026-04-25",
		Title:   "preexisting",
	}, []byte("body\n"))
	store.Seed(pre, time.Unix(1, 0))

	repo := &testutil.FakeRepoDetector{Info: ports.RepoInfo{
		Root: "/repos/bar",
		URL:  domain.CanonicalURL("github.com/foo/bar"),
	}}
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)}

	u := usecase.NewCreateProject(store, repo, clock, &testutil.FakeEditor{})
	out, err := u.Execute(context.Background(), usecase.CreateProjectInput{Cwd: "/repos/bar"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Created {
		t.Error("expected Created=false")
	}
	got, _ := store.Get(context.Background(), id)
	if got.Meta.Title != "preexisting" {
		t.Errorf("existing note overwritten, Title=%q", got.Meta.Title)
	}
}

func TestCreateProject_ExistsError(t *testing.T) {
	t.Parallel()
	forced := errors.New("exists boom")
	store := testutil.NewFakeNoteStore()
	store.ExistsErr = forced
	repo := &testutil.FakeRepoDetector{Info: ports.RepoInfo{URL: "github.com/foo/bar"}}

	u := usecase.NewCreateProject(store, repo, testutil.FixedClock{Time: time.Now().UTC()}, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateProjectInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestCreateProject_PutError(t *testing.T) {
	t.Parallel()
	forced := errors.New("put boom")
	store := testutil.NewFakeNoteStore()
	store.PutErr = forced
	repo := &testutil.FakeRepoDetector{Info: ports.RepoInfo{URL: "github.com/foo/bar"}}

	u := usecase.NewCreateProject(store, repo, testutil.FixedClock{Time: time.Now().UTC()}, &testutil.FakeEditor{})
	_, err := u.Execute(context.Background(), usecase.CreateProjectInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestCreateProject_EditorError(t *testing.T) {
	t.Parallel()
	forced := errors.New("editor down")
	editor := &testutil.FakeEditor{Err: forced}
	repo := &testutil.FakeRepoDetector{Info: ports.RepoInfo{URL: "github.com/foo/bar"}}

	u := usecase.NewCreateProject(testutil.NewFakeNoteStore(), repo, testutil.FixedClock{Time: time.Now().UTC()}, editor)
	_, err := u.Execute(context.Background(), usecase.CreateProjectInput{})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
