package cli_test

import (
	"strings"
	"testing"
)

func TestPath_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "path", "daily/2026-04-25")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !strings.Contains(stdout, "/fake-notebook/daily/2026-04-25.md") {
		t.Errorf("path got %q", stdout)
	}
}

func TestPath_BadID(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "path", "../escape")
	if err == nil {
		t.Fatal("expected error for bad ID")
	}
}
