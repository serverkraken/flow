package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestWrite_HelpShowsLong(t *testing.T) {
	t.Parallel()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	if err := Execute([]string{"write", "--help"}, out, errOut, Deps{Repo: &testutil.FakeRepoDetector{}}); err != nil {
		t.Fatalf("write --help: %v", err)
	}
	if !strings.Contains(out.String(), "small picker") {
		t.Errorf("missing write Long in --help: %q", out.String())
	}
}

func TestWrite_DailyChoiceCallsCreateDaily(t *testing.T) {
	t.Cleanup(swapRunPicker(func(_ context.Context, _ bool) (writepicker.Result, error) {
		return writepicker.Result{Choice: writepicker.ChoiceDaily}, nil
	}))

	store := testutil.NewFakeNoteStore()
	deps := buildWriteDeps(store)

	out, _, err := runCmdInternal(t, deps, "write", "--cwd", "/x")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "daily/2026-04-25") {
		t.Errorf("expected daily ID in output, got %q", out)
	}
}

func TestWrite_ProjectChoiceCallsCreateProject(t *testing.T) {
	t.Cleanup(swapRunPicker(func(_ context.Context, allowProject bool) (writepicker.Result, error) {
		if !allowProject {
			t.Errorf("picker called with allowProject=false but a repo was detected")
		}
		return writepicker.Result{Choice: writepicker.ChoiceProject}, nil
	}))

	store := testutil.NewFakeNoteStore()
	deps, repo := buildWriteDepsWithRepo(store)
	// Mutate the same repo pointer the CreateProject use case captured;
	// replacing deps.Repo with a new pointer would leave the use case
	// holding the original empty one.
	repo.Info = ports.RepoInfo{URL: "github.com/foo/bar"}

	out, _, err := runCmdInternal(t, deps, "write", "--cwd", "/repos/foo")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "projects/github.com/foo/bar") {
		t.Errorf("expected project ID in output, got %q", out)
	}
}

func TestWrite_FreeChoiceCallsCreateFree(t *testing.T) {
	t.Cleanup(swapRunPicker(func(_ context.Context, _ bool) (writepicker.Result, error) {
		return writepicker.Result{Choice: writepicker.ChoiceFree, Slug: "setup"}, nil
	}))

	store := testutil.NewFakeNoteStore()
	deps := buildWriteDeps(store)

	out, _, err := runCmdInternal(t, deps, "write", "--cwd", "/x")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "notes/setup") {
		t.Errorf("expected notes/setup in output, got %q", out)
	}
}

func TestWrite_CancelDoesNothing(t *testing.T) {
	t.Cleanup(swapRunPicker(func(_ context.Context, _ bool) (writepicker.Result, error) {
		return writepicker.Result{Choice: writepicker.ChoiceCancel}, nil
	}))

	store := testutil.NewFakeNoteStore()
	deps := buildWriteDeps(store)

	out, _, err := runCmdInternal(t, deps, "write", "--cwd", "/x")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if out != "" {
		t.Errorf("cancel must produce no output, got %q", out)
	}
}

func TestWrite_PickerErrorPropagates(t *testing.T) {
	forced := errors.New("forced picker err")
	t.Cleanup(swapRunPicker(func(_ context.Context, _ bool) (writepicker.Result, error) {
		return writepicker.Result{}, forced
	}))

	_, _, err := runCmdInternal(t, buildWriteDeps(testutil.NewFakeNoteStore()), "write", "--cwd", "/x")
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestWrite_UnknownChoiceErrors(t *testing.T) {
	t.Cleanup(swapRunPicker(func(_ context.Context, _ bool) (writepicker.Result, error) {
		return writepicker.Result{Choice: writepicker.Choice(99)}, nil
	}))

	_, _, err := runCmdInternal(t, buildWriteDeps(testutil.NewFakeNoteStore()), "write", "--cwd", "/x")
	if err == nil {
		t.Fatal("expected error for unknown choice")
	}
}

// --- helpers ----------------------------------------------------------------

func runCmdInternal(t *testing.T, deps Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	err = Execute(args, out, errOut, deps)
	return out.String(), errOut.String(), err
}

func swapRunPicker(fn func(context.Context, bool) (writepicker.Result, error)) func() {
	prev := runPicker
	runPicker = fn
	return func() { runPicker = prev }
}

func buildWriteDeps(store *testutil.FakeNoteStore) Deps {
	deps, _ := buildWriteDepsWithRepo(store)
	return deps
}

func buildWriteDepsWithRepo(store *testutil.FakeNoteStore) (Deps, *testutil.FakeRepoDetector) {
	editor := &testutil.FakeEditor{}
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)}
	repo := &testutil.FakeRepoDetector{}
	deps := Deps{
		Store:         store,
		Repo:          repo,
		CreateDaily:   usecase.NewCreateDaily(store, clock, editor),
		CreateProject: usecase.NewCreateProject(store, repo, clock, editor),
		CreateFree:    usecase.NewCreateFree(store, editor),
	}
	return deps, repo
}

// Silence unused imports if a particular build drops them.
var (
	_ = domain.NoteType("")
)
