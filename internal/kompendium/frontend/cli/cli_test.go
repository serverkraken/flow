package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/cli"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// testEnv bundles the fakes a test typically needs to drive the CLI. Each
// constructor returns a fresh instance so tests stay independent.
type testEnv struct {
	store  *testutil.FakeNoteStore
	index  *testutil.FakeIndexer
	repo   *testutil.FakeRepoDetector
	editor *testutil.FakeEditor
	git    *testutil.FakeNotebookInit
	tar    *testutil.FakeTarSnapshot
	bundle *testutil.FakeNotebookBundle
	legacy *testutil.FakeLegacySource
	remote *fakeRemote
	deps   cli.Deps
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store := testutil.NewFakeNoteStore()
	index := testutil.NewFakeIndexer()
	repo := &testutil.FakeRepoDetector{}
	editor := &testutil.FakeEditor{}
	git := &testutil.FakeNotebookInit{}
	tar := &testutil.FakeTarSnapshot{}
	bundle := &testutil.FakeNotebookBundle{}
	legacy := &testutil.FakeLegacySource{}
	remote := &fakeRemote{}
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)}

	return &testEnv{
		store:  store,
		index:  index,
		repo:   repo,
		editor: editor,
		git:    git,
		tar:    tar,
		bundle: bundle,
		legacy: legacy,
		remote: remote,
		deps: cli.Deps{
			Store:            store,
			Repo:             repo,
			CreateDaily:      usecase.NewCreateDaily(store, clock, editor),
			CreateProject:    usecase.NewCreateProject(store, repo, clock, editor),
			CreateFree:       usecase.NewCreateFree(store, editor),
			CaptureDaily:     usecase.NewCaptureDaily(store, clock),
			Open:             usecase.NewOpen(store, editor),
			ListNotes:        usecase.NewListNotes(store),
			SearchNotes:      usecase.NewSearchNotes(index),
			RenderDaily:      usecase.NewRenderDaily(store),
			RenderBacklinks:  usecase.NewRenderBacklinks(store, index),
			InitNotebook:     usecase.NewInitNotebook(store, git),
			SnapshotNotebook: usecase.NewSnapshotNotebook(store, git),
			ExportTar:        usecase.NewExportTar(store, tar),
			ImportTar:        usecase.NewImportTar(store, tar),
			ExportBundle:     usecase.NewExportBundle(store, bundle),
			ImportBundle:     usecase.NewImportBundle(store, bundle),
			SyncNotebook:     usecase.NewSyncNotebook(store, remote),
			ManageRemote:     usecase.NewManageRemote(store, remote),
			Doctor:           usecase.NewDoctor(store, git),
			ImportLegacy:     usecase.NewImportLegacy(store, legacy),
			RebuildIndex:     usecase.NewRebuildIndex(store, index),
			DeleteNote:       usecase.NewDeleteNote(store, index),
		},
	}
}

// fakeRemote is the in-test ports.NotebookRemote driving CLI tests for
// `kompendium sync` / `kompendium remote`. Lives here so it doesn't get
// confused with the production gitsnapshot.Manager.
type fakeRemote struct {
	url     string
	getErr  error
	setURL  string
	setErr  error
	stats   ports.SyncStats
	syncErr error
}

func (f *fakeRemote) GetRemote(_ context.Context, _ string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.url, nil
}

func (f *fakeRemote) SetRemote(_ context.Context, _, url string) error {
	f.setURL = url
	return f.setErr
}

func (f *fakeRemote) Sync(_ context.Context, _ string) (ports.SyncStats, error) {
	return f.stats, f.syncErr
}

var _ ports.NotebookRemote = (*fakeRemote)(nil)

func runCmd(t *testing.T, deps cli.Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	err = cli.Execute(args, out, errOut, deps)
	return out.String(), errOut.String(), err
}

func mustNote(t *testing.T, id domain.ID, typ domain.NoteType, project, title, body string) domain.Note {
	t.Helper()
	fm := domain.Frontmatter{
		ID:      id.String(),
		Type:    typ,
		Project: project,
		Title:   title,
	}
	if typ == domain.TypeDaily {
		fm.Date = "2026-04-25"
	}
	n, err := domain.NewNote(id, fm, []byte(body))
	if err != nil {
		t.Fatalf("NewNote(%q): %v", id, err)
	}
	return n
}
