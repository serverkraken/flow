package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestSync_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.remote.URL = "git@github.com:foo/bar.git"
	env.remote.Stats = ports.SyncStats{Pulled: true, Pushed: true}

	stdout, _, err := runCmd(t, env.deps, "sync")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(stdout, "Synced") {
		t.Errorf("expected sync acknowledgement, got %q", stdout)
	}
}

func TestSync_NoRemoteHint(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.remote.SyncErr = ports.ErrNoRemoteConfigured

	_, _, err := runCmd(t, env.deps, "sync")
	if err == nil {
		t.Fatal("expected error when no remote is configured")
	}
	if !strings.Contains(err.Error(), "remote set") {
		t.Errorf("error should hint at `remote set`, got %v", err)
	}
}

func TestSync_PullSucceededPushFailed(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced push error")
	env.remote.Stats = ports.SyncStats{Pulled: true}
	env.remote.SyncErr = forced

	_, _, err := runCmd(t, env.deps, "sync")
	if err == nil {
		t.Fatal("expected propagated error when push fails")
	}
}

func TestRemote_PrintsNoneWhenUnset(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.remote.GetErr = ports.ErrNoRemoteConfigured

	stdout, _, err := runCmd(t, env.deps, "remote")
	if err != nil {
		t.Fatalf("remote: %v", err)
	}
	if !strings.Contains(stdout, "(none)") {
		t.Errorf("expected `(none)` when remote unset, got %q", stdout)
	}
}

func TestRemote_PrintsURL(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.remote.URL = "git@github.com:foo/bar.git"

	stdout, _, err := runCmd(t, env.deps, "remote")
	if err != nil {
		t.Fatalf("remote: %v", err)
	}
	if !strings.Contains(stdout, "git@github.com:foo/bar.git") {
		t.Errorf("URL not printed, got %q", stdout)
	}
}

func TestRemote_Set(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	stdout, _, err := runCmd(t, env.deps, "remote", "set", "https://example.test/notes.git")
	if err != nil {
		t.Fatalf("remote set: %v", err)
	}
	if !strings.Contains(stdout, "https://example.test/notes.git") {
		t.Errorf("missing URL in confirmation, got %q", stdout)
	}
	if env.remote.SetURL != "https://example.test/notes.git" {
		t.Errorf("SetRemote not called with right URL, got %q", env.remote.SetURL)
	}
}

func TestRemote_SetRequiresArg(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "remote", "set")
	if err == nil {
		t.Fatal("expected cobra arity error")
	}
}
