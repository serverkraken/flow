package cli_test

import (
	"errors"
	"strings"
	"testing"
)

func TestSnapshot_CleanTree(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.HasChangesValue = false

	stdout, _, err := runCmd(t, env.deps, "snapshot")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !strings.Contains(stdout, "Nothing to snapshot") {
		t.Errorf("output got %q", stdout)
	}
	if len(env.git.Snapshots) != 0 {
		t.Errorf("Snapshot must not be called on clean tree, got %+v", env.git.Snapshots)
	}
}

func TestSnapshot_Dirty(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.HasChangesValue = true

	stdout, _, err := runCmd(t, env.deps, "snapshot", "-m", "manual save")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !strings.Contains(stdout, "Snapshot committed") {
		t.Errorf("output got %q", stdout)
	}
	if len(env.git.Snapshots) != 1 || env.git.Snapshots[0] != "manual save" {
		t.Errorf("snapshot message lost, got %+v", env.git.Snapshots)
	}
}

func TestSnapshot_GitError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced has-changes")
	env.git.HasChangesErr = forced

	_, _, err := runCmd(t, env.deps, "snapshot")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
