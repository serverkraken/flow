package main

// Tests for the kompendium → worktime adapter shims. main.go itself is
// composition-only (excluded from the per-package coverage budget by
// the Makefile rationale), but the adapter in note_lister_adapter.go is
// real translation logic that the worktime screen depends on.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	flowtheme "github.com/serverkraken/flow/internal/frontend/tui/theme"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompendiumcli "github.com/serverkraken/flow/internal/kompendium/frontend/cli"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	komptestutil "github.com/serverkraken/flow/internal/kompendium/testutil"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
)

func mustNote(t *testing.T, id, title string, typ kompdomain.NoteType, project string) kompdomain.Note {
	t.Helper()
	fm := kompdomain.Frontmatter{ID: id, Type: typ, Title: title, Project: project}
	n, err := kompdomain.NewNote(kompdomain.ID(id), fm, []byte(""))
	if err != nil {
		t.Fatalf("NewNote(%s): %v", id, err)
	}
	return n
}

func TestKompendiumNoteLister_NilReturnsEmpty(t *testing.T) {
	t.Parallel()
	var l *kompendiumNoteLister
	if got := l.Recent(10); got != nil {
		t.Errorf("nil receiver should return nil, got %+v", got)
	}
}

func TestKompendiumNoteLister_NilListNotesReturnsNil(t *testing.T) {
	t.Parallel()
	l := newKompendiumNoteLister(kompendiumcli.Deps{}, kompdomain.CanonicalURL(""))
	if got := l.Recent(10); got != nil {
		t.Errorf("nil ListNotes should return nil, got %+v", got)
	}
}

func TestKompendiumNoteLister_ZeroLimitReturnsNil(t *testing.T) {
	t.Parallel()
	store := komptestutil.NewFakeNoteStore()
	deps := kompendiumcli.Deps{ListNotes: kompusecase.NewListNotes(store)}
	l := newKompendiumNoteLister(deps, kompdomain.CanonicalURL(""))
	if got := l.Recent(0); got != nil {
		t.Errorf("limit=0 should return nil, got %+v", got)
	}
}

func TestKompendiumNoteLister_RecentMapsTitleAndID(t *testing.T) {
	t.Parallel()
	store := komptestutil.NewFakeNoteStore()
	n1 := mustNote(t, "daily/2026-05-01", "First", kompdomain.TypeDaily, "")
	n2 := mustNote(t, "notes/x", "", kompdomain.TypeFree, "")
	store.Seed(n1, time.Now())
	store.Seed(n2, time.Now().Add(-time.Hour))
	deps := kompendiumcli.Deps{ListNotes: kompusecase.NewListNotes(store)}
	l := newKompendiumNoteLister(deps, kompdomain.CanonicalURL(""))
	got := l.Recent(5)
	if len(got) != 2 {
		t.Fatalf("Recent: got %d entries, want 2", len(got))
	}
	// n1 has a title → it should win; n2 has no title → falls back to id.
	titles := map[string]string{}
	for _, s := range got {
		titles[s.ID] = s.Title
	}
	if titles["daily/2026-05-01"] != "First" {
		t.Errorf("titled note: %q, want First", titles["daily/2026-05-01"])
	}
	if titles["notes/x"] != "notes/x" {
		t.Errorf("titleless note should fall back to id, got %q", titles["notes/x"])
	}
}

// Forced-error store proves the err-swallow branch (Recent returns nil
// rather than propagating an error to its caller).
type erroringStore struct {
	*komptestutil.FakeNoteStore
}

func (e *erroringStore) List(_ context.Context, _ kompports.ListFilter) ([]kompports.NoteEntry, error) {
	return nil, errors.New("boom")
}

func TestKompendiumNoteLister_SwallowsErrors(t *testing.T) {
	t.Parallel()
	s := &erroringStore{FakeNoteStore: komptestutil.NewFakeNoteStore()}
	deps := kompendiumcli.Deps{ListNotes: kompusecase.NewListNotes(s)}
	l := newKompendiumNoteLister(deps, kompdomain.CanonicalURL(""))
	if got := l.Recent(5); got != nil {
		t.Errorf("error should be swallowed → nil result, got %+v", got)
	}
}

// — detectCurrentRepo —

func TestDetectCurrentRepo_NilRepoReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := detectCurrentRepo(kompendiumcli.Deps{}); got != "" {
		t.Errorf("nil repo should yield empty URL, got %q", got)
	}
}

func TestDetectCurrentRepo_DetectorErrReturnsEmpty(t *testing.T) {
	t.Parallel()
	deps := kompendiumcli.Deps{Repo: &komptestutil.FakeRepoDetector{Err: errors.New("not a repo")}}
	if got := detectCurrentRepo(deps); got != "" {
		t.Errorf("detect error should yield empty URL, got %q", got)
	}
}

func TestDetectCurrentRepo_HappyPath(t *testing.T) {
	t.Parallel()
	deps := kompendiumcli.Deps{Repo: &komptestutil.FakeRepoDetector{
		Info: kompports.RepoInfo{URL: "github.com/foo/bar"},
	}}
	if got := detectCurrentRepo(deps); !strings.Contains(string(got), "foo/bar") {
		t.Errorf("happy path should yield repo URL, got %q", got)
	}
}

// — kompendiumNoteReader —

func TestKompendiumNoteReader_HappyPath(t *testing.T) {
	t.Parallel()
	store := komptestutil.NewFakeNoteStore()
	n := mustNote(t, "notes/x", "X", kompdomain.TypeFree, "")
	n.Body = []byte("# Hello\nworld")
	store.Seed(n, time.Now())
	r := newKompendiumNoteReader(kompendiumcli.Deps{Store: store})
	body, err := r.Read("notes/x")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(body, "Hello") {
		t.Errorf("body should include note content, got %q", body)
	}
}

func TestKompendiumNoteReader_InvalidIDReturnsError(t *testing.T) {
	t.Parallel()
	store := komptestutil.NewFakeNoteStore()
	r := newKompendiumNoteReader(kompendiumcli.Deps{Store: store})
	if _, err := r.Read("--bad--"); err == nil {
		t.Errorf("invalid id should error")
	}
}

func TestKompendiumNoteReader_MissingNoteReturnsError(t *testing.T) {
	t.Parallel()
	store := komptestutil.NewFakeNoteStore()
	r := newKompendiumNoteReader(kompendiumcli.Deps{Store: store})
	if _, err := r.Read("notes/missing"); err == nil {
		t.Errorf("missing note should error")
	}
}

// — parseEnvHoursDuration —

func TestParseEnvHoursDuration_Empty(t *testing.T) {
	t.Setenv("FLOW_TEST_HOURS", "")
	if got := parseEnvHoursDuration("FLOW_TEST_HOURS"); got != 0 {
		t.Errorf("empty env: got %v, want 0", got)
	}
}

func TestParseEnvHoursDuration_Valid(t *testing.T) {
	t.Setenv("FLOW_TEST_HOURS", "8")
	got := parseEnvHoursDuration("FLOW_TEST_HOURS")
	if got != 8*time.Hour {
		t.Errorf("8 → %v, want 8h", got)
	}
}

func TestParseEnvHoursDuration_Decimal(t *testing.T) {
	t.Setenv("FLOW_TEST_HOURS", "7.5")
	got := parseEnvHoursDuration("FLOW_TEST_HOURS")
	if got != 7*time.Hour+30*time.Minute {
		t.Errorf("7.5 → %v, want 7h30m", got)
	}
}

func TestParseEnvHoursDuration_Garbage(t *testing.T) {
	t.Setenv("FLOW_TEST_HOURS", "not-a-number")
	if got := parseEnvHoursDuration("FLOW_TEST_HOURS"); got != 0 {
		t.Errorf("garbage: %v, want 0", got)
	}
}

func TestParseEnvHoursDuration_NonPositive(t *testing.T) {
	t.Setenv("FLOW_TEST_HOURS", "-1")
	if got := parseEnvHoursDuration("FLOW_TEST_HOURS"); got != 0 {
		t.Errorf("negative: %v, want 0", got)
	}
	t.Setenv("FLOW_TEST_HOURS", "0")
	if got := parseEnvHoursDuration("FLOW_TEST_HOURS"); got != 0 {
		t.Errorf("zero: %v, want 0", got)
	}
}

// — buildKompendiumDeps —

func TestBuildKompendiumDeps_WiresAllUseCases(t *testing.T) {
	t.Parallel()
	notebookDir := t.TempDir()
	indexDir := t.TempDir()
	p := Paths{
		KompendiumNotebook: notebookDir,
		KompendiumIndex:    filepath.Join(indexDir, "index.db"),
	}
	clock := systemclock.New()
	deps, cleanup, err := buildKompendiumDeps(p, clock)
	if err != nil {
		t.Fatalf("buildKompendiumDeps: %v", err)
	}
	t.Cleanup(cleanup)
	if deps.Store == nil || deps.ListNotes == nil || deps.SearchNotes == nil {
		t.Errorf("missing required dep wiring: %+v", deps)
	}
	if deps.CreateDaily == nil || deps.CreateProject == nil || deps.CreateFree == nil {
		t.Errorf("missing create-use-case wiring")
	}
	if deps.ImportLegacy == nil || deps.RebuildIndex == nil || deps.DeleteNote == nil {
		t.Errorf("missing maintenance-use-case wiring")
	}
}

// — buildDeps full graph —

func TestBuildDeps_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	tmuxDir := filepath.Join(tmp, ".tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(tmp, "cache")
	stateDir := filepath.Join(tmp, "state")
	pluginsDir := filepath.Join(tmp, "plugins")
	sourceRoot := filepath.Join(tmp, "Source")
	notebook := filepath.Join(tmp, "notes")
	index := filepath.Join(tmp, "index.db")
	for _, d := range []string{cacheDir, stateDir, pluginsDir, sourceRoot, notebook} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := Paths{
		Home:               tmp,
		WorktimeLog:        filepath.Join(tmuxDir, "worktime.log"),
		TmuxDir:            tmuxDir,
		CacheDir:           cacheDir,
		PluginsDir:         pluginsDir,
		StateDir:           stateDir,
		Cheatsheet:         filepath.Join(tmuxDir, "cheatsheet.md"),
		SourceCodeRoot:     sourceRoot,
		KompendiumNotebook: notebook,
		KompendiumIndex:    index,
	}
	deps, cleanup, err := buildDeps(context.Background(), p, Env{WorktimeTargetHours: 8 * time.Hour, WorktimeLand: "BE", ServerURL: "http://localhost:0"})
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	t.Cleanup(cleanup)
	if deps.Worktime.Reader == nil || deps.Sidekick.FlowState == nil {
		t.Errorf("required dep wiring missing")
	}
	if deps.Cheatsheet.Reader == nil || deps.Palette.Screen == nil {
		t.Errorf("standalone wiring missing")
	}
	// Invoke every screen factory so buildDeps's closures are exercised
	// (not just defined). Each factory builds a tea.Model — we drop them
	// immediately; the call is the assertion.
	pal := flowtheme.Load()
	factories := []func(flowtheme.Palette) tea.Model{
		deps.Sidekick.Cheatsheet,
		deps.Sidekick.Palette,
		deps.Sidekick.Projects,
		deps.Sidekick.Worktime,
		deps.Sidekick.Notes,
		deps.Worktime.Screen,
		deps.Palette.Screen,
		deps.Projects.Screen,
	}
	for i, factory := range factories {
		if m := factory(pal); m == nil {
			t.Errorf("factory %d returned nil", i)
		}
	}
}

// buildNotesScreen wires the kompendium browse Bubbletea model into the
// sidekick. The factory has no return-value to assert beyond non-nil;
// the relevant proof is that the wiring runs (SetPalette calls,
// indexer-age + backlinks options, WithIndexAge / WithBacklinks branches).
func TestBuildNotesScreen_Construct(t *testing.T) {
	notebook := t.TempDir()
	index := filepath.Join(t.TempDir(), "index.db")
	p := Paths{KompendiumNotebook: notebook, KompendiumIndex: index}
	deps, cleanup, err := buildKompendiumDeps(p, systemclock.New())
	if err != nil {
		t.Fatalf("buildKompendiumDeps: %v", err)
	}
	t.Cleanup(cleanup)
	// Seed the index file so the WithIndexAge stat returns a non-zero time.
	if err := os.WriteFile(index, []byte("seeded"), 0o600); err != nil {
		t.Fatal(err)
	}
	pal := flowtheme.Load()
	model := buildNotesScreen(p, pal, deps, kompdomain.CanonicalURL(""))
	if model == nil {
		t.Errorf("buildNotesScreen should return a non-nil model")
	}
}

func TestBuildKompendiumDeps_BadNotebookPathFails(t *testing.T) {
	t.Parallel()
	// Pass a path inside a non-writable file (using a regular file as
	// the "directory" — fsstore.New should fail to create it).
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Paths{
		KompendiumNotebook: filepath.Join(blocker, "subdir"),
		KompendiumIndex:    filepath.Join(tmp, "index.db"),
	}
	if _, _, err := buildKompendiumDeps(p, systemclock.New()); err == nil {
		t.Errorf("expected error when notebook dir cannot be created")
	}
}
