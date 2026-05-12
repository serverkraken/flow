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
	remote := &testutil.FakeNotebookRemote{Stats: ports.SyncStats{Pulled: true, Pushed: true}}
	u := usecase.NewSyncNotebook(store, remote)

	out, err := u.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Stats.Pulled || !out.Stats.Pushed {
		t.Errorf("stats got %+v, want both flags true", out.Stats)
	}
	if remote.SyncRoot != store.Root() {
		t.Errorf("Sync got root %q, want %q", remote.SyncRoot, store.Root())
	}
}

func TestSyncNotebook_RemoteErrorWrapped(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	remote := &testutil.FakeNotebookRemote{SyncErr: ports.ErrNoRemoteConfigured}
	u := usecase.NewSyncNotebook(store, remote)

	_, err := u.Execute(context.Background())
	if !errors.Is(err, ports.ErrNoRemoteConfigured) {
		t.Errorf("got %v, want ErrNoRemoteConfigured", err)
	}
}

func TestManageRemote_GetSet(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	remote := &testutil.FakeNotebookRemote{}
	u := usecase.NewManageRemote(store, remote)

	// Initially empty (FakeNotebookRemote.URL == "") → ErrNoRemoteConfigured →
	// translated to URL: "" by Get.
	remote.GetErr = ports.ErrNoRemoteConfigured
	got, err := u.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "" {
		t.Errorf("URL got %q, want empty", got.URL)
	}

	// Set a remote.
	remote.GetErr = nil
	if _, err := u.Set(context.Background(), usecase.SetInput{URL: "git@github.com:foo/bar.git"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if remote.SetURL != "git@github.com:foo/bar.git" {
		t.Errorf("Set got %q", remote.SetURL)
	}

	// Read it back.
	remote.URL = "git@github.com:foo/bar.git"
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
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &testutil.FakeNotebookRemote{})
	_, err := u.Set(context.Background(), usecase.SetInput{URL: "  "})
	if !errors.Is(err, usecase.ErrRemoteURLRequired) {
		t.Errorf("got %v, want ErrRemoteURLRequired", err)
	}
}

func TestManageRemote_GetErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced get-remote error")
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &testutil.FakeNotebookRemote{GetErr: forced})
	_, err := u.Get(context.Background())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}

func TestManageRemote_SetErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced set-remote error")
	u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), &testutil.FakeNotebookRemote{SetErr: forced})
	_, err := u.Set(context.Background(), usecase.SetInput{URL: "https://example.test/notes.git"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}
