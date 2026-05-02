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
	env.remote.url = "git@github.com:foo/bar.git"
	env.remote.stats = ports.SyncStats{Pulled: true, Pushed: true}

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
	env.remote.syncErr = ports.ErrNoRemoteConfigured

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
	env.remote.stats = ports.SyncStats{Pulled: true}
	env.remote.syncErr = forced

	_, _, err := runCmd(t, env.deps, "sync")
	if err == nil {
		t.Fatal("expected propagated error when push fails")
	}
}

func TestRemote_PrintsNoneWhenUnset(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.remote.getErr = ports.ErrNoRemoteConfigured

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
	env.remote.url = "git@github.com:foo/bar.git"

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
	if env.remote.setURL != "https://example.test/notes.git" {
		t.Errorf("SetRemote not called with right URL, got %q", env.remote.setURL)
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
