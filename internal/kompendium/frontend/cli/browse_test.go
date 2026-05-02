package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// Internal test (package cli, not cli_test) so we can swap the package-level
// runBrowse hook without exposing it through a public API.

func TestBrowse_HelpListsTheSubcommand(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	deps := Deps{
		ListNotes: usecase.NewListNotes(store),
		Repo:      &testutil.FakeRepoDetector{},
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	if err := Execute([]string{"browse", "--help"}, out, errOut, deps); err != nil {
		t.Fatalf("browse --help: %v", err)
	}
	if !strings.Contains(out.String(), "interactive browser") {
		t.Errorf("missing browse Long in --help output: %q", out.String())
	}
	if !strings.Contains(out.String(), "--cwd") {
		t.Errorf("missing --cwd flag in --help output: %q", out.String())
	}
}

func TestBrowse_RunsWithDetectedRepo(t *testing.T) {
	// Mutates package-level runBrowse, so cannot run in parallel with the
	// other browse tests that do the same.
	t.Cleanup(swapRunBrowse(func(_ context.Context, _ Deps, cwd string) error {
		if cwd == "" {
			t.Errorf("runBrowse received empty cwd")
		}
		return nil
	}))

	deps := Deps{
		Repo: &testutil.FakeRepoDetector{Info: ports.RepoInfo{URL: "github.com/foo/bar"}},
	}
	if err := Execute([]string{"browse", "--cwd", "/repos/foo"}, &bytes.Buffer{}, &bytes.Buffer{}, deps); err != nil {
		t.Fatalf("browse: %v", err)
	}
}

func TestBrowse_PropagatesProgramError(t *testing.T) {
	forced := errors.New("forced tea program error")
	t.Cleanup(swapRunBrowse(func(_ context.Context, _ Deps, _ string) error {
		return forced
	}))

	err := Execute([]string{"browse", "--cwd", "/x"}, &bytes.Buffer{}, &bytes.Buffer{}, Deps{
		Repo: &testutil.FakeRepoDetector{},
	})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestBrowse_CwdFallsBackToOsGetwd(t *testing.T) {
	var captured string
	t.Cleanup(swapRunBrowse(func(_ context.Context, _ Deps, cwd string) error {
		captured = cwd
		return nil
	}))

	if err := Execute([]string{"browse"}, &bytes.Buffer{}, &bytes.Buffer{}, Deps{
		Repo: &testutil.FakeRepoDetector{},
	}); err != nil {
		t.Fatal(err)
	}
	if captured == "" {
		t.Error("expected cwd to be resolved via os.Getwd, got empty")
	}
}

// --- helpers ----------------------------------------------------------------

func swapRunBrowse(fn func(context.Context, Deps, string) error) func() {
	prev := runBrowse
	runBrowse = fn
	return func() { runBrowse = prev }
}

// Silence unused-import warnings if a test stops referencing these symbols.
var (
	_ = domain.NoteType("")
	_ = time.Now
	_ = strings.Builder{}
)
