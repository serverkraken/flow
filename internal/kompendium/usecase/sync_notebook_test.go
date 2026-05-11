package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestSyncNotebook_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	remote := &fakeRemote{stats: ports.SyncStats{Pulled: true, Pushed: true}}
	u := usecase.NewSyncNotebook(store, remote)

	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Stats.Pulled || !out.Stats.Pushed {
		t.Errorf("stats got %+v, want both flags true", out.Stats)
	}
	if remote.syncRoot != store.Root() {
		t.Errorf("Sync got root %q, want %q", remote.syncRoot, store.Root())
	}
}

func TestSyncNotebook_RemoteErrorWrapped(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	remote := &fakeRemote{syncErr: ports.ErrNoRemoteConfigured}
	u := usecase.NewSyncNotebook(store, remote)

	_, err := u.Execute(context.Background())
	if !errors.Is(err, ports.ErrNoRemoteConfigured) {
		t.Errorf("got %v, want ErrNoRemoteConfigured", err)
	}
}

func TestManageRemote_GetSet(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	remote := &fakeRemote{}
	u := usecase.NewManageRemote(store, remote)

	// Initially empty (fakeRemote.url == "") → ErrNoRemoteConfigured →
	// translated to URL: "" by Get.
	remote.getErr = ports.ErrNoRemoteConfigured
	got, err := u.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "" {
		t.Errorf("URL got %q, want empty", got.URL)
	}

	// Set a remote.
	remote.getErr = nil
	if _, err := u.Set(context.Background(), usecase.SetInput{URL: "git@github.com:foo/bar.git"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if remote.setURL != "git@github.com:foo/bar.git" {
		t.Errorf("Set got %q", remote.setURL)
	}

	// Read it back.
	remote.url = "git@github.com:foo/bar.git"
	got, err = u.Get(context.Background())
	if err != nil {
		t.Fatalf("Get post-set: %v", err)
	}
	if got.URL != "git@github.com:foo/bar.git" {
		t.Errorf("URL got %q after set", got.URL)
	}
}

func TestManageRemote_SetRequiresURL(t *testing.T) {
	t.Parallel()
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &fakeRemote{})
	_, err := u.Set(context.Background(), usecase.SetInput{URL: "  "})
	if !errors.Is(err, usecase.ErrRemoteURLRequired) {
		t.Errorf("got %v, want ErrRemoteURLRequired", err)
	}
}

func TestManageRemote_GetErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced get-remote error")
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &fakeRemote{getErr: forced})
	_, err := u.Get(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}

func TestManageRemote_SetErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced set-remote error")
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &fakeRemote{setErr: forced})
	_, err := u.Set(context.Background(), usecase.SetInput{URL: "https://example.test/notes.git"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}

// fakeRemote is an in-test ports.NotebookRemote. Lives next to the use-
// case tests since no other package needs it (production sync gets a
// real gitsnapshot.Manager via main.go).
type fakeRemote struct {
	url      string
	getErr   error
	setURL   string
	setErr   error
	syncRoot string
	stats    ports.SyncStats
	syncErr  error
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

func (f *fakeRemote) Sync(_ context.Context, root string) (ports.SyncStats, error) {
	f.syncRoot = root
	return f.stats, f.syncErr
}

var _ ports.NotebookRemote = (*fakeRemote)(nil)
