package cli_test

import (
	"errors"
	"strings"
	"testing"
)

func TestInit_FreshDirectory(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	stdout, _, err := runCmd(t, env.deps, "init")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.HasPrefix(stdout, "Initialised") {
		t.Errorf("output got %q", stdout)
	}
	if !env.git.Initialized {
		t.Error("Init must have been called on the FakeNotebookInit")
	}
}

func TestInit_AlreadyARepo(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true

	stdout, _, err := runCmd(t, env.deps, "init")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.HasPrefix(stdout, "Already a git repo") {
		t.Errorf("output got %q", stdout)
	}
	if env.git.Initialized {
		t.Error("Init must not be called on an existing repo")
	}
}

func TestInit_GitError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced is-repo")
	env.git.IsRepoErr = forced

	_, _, err := runCmd(t, env.deps, "init")
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}
