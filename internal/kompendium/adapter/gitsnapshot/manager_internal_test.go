package gitsnapshot

import (
	"context"
	"errors"
	"testing"
)

// Mock-runner tests cover the non-exit error branches that real git can't
// reproduce (binary missing, exec failure pre-spawn).

func TestIsRepo_NonExitErr(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced non-exit error")
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", forced
	}}
	_, err := m.IsRepo(context.Background(), "/x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err must wrap forced, got %v", err)
	}
}

func TestInit_GitInitFails(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced init err")
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", forced
	}}
	err := m.Init(context.Background(), "/x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err must wrap forced, got %v", err)
	}
}

func TestInit_AddFails(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced add err")
	var calls int
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		calls++
		if calls == 1 { // git init
			return "", nil
		}
		return "", forced // git add
	}}
	// Use a real tempdir — Init now writes a .gitignore between `git init`
	// and `git add`, so the root must actually exist on disk.
	err := m.Init(context.Background(), t.TempDir())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestInit_CommitFails(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced commit err")
	var calls int
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		calls++
		if calls < 3 { // init, add succeed
			return "", nil
		}
		return "", forced // commit
	}}
	err := m.Init(context.Background(), t.TempDir())
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestSnapshot_AddFails(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced add err")
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", forced
	}}
	err := m.Snapshot(context.Background(), "/x", "msg")
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}

func TestHasUncommittedChanges_RunErr(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced status err")
	m := Manager{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", forced
	}}
	_, err := m.HasUncommittedChanges(context.Background(), "/x")
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}
