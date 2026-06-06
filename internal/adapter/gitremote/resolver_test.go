package gitremote_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/gitremote"
)

// gitOnPath reports whether the `git` binary is on PATH. Tests skip when
// it isn't — CI without git is a legitimate environment.
func gitOnPath(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("git")
	return err == nil
}

func TestResolver_RemoteURL_NoRepo_FalseOK(t *testing.T) {
	if !gitOnPath(t) {
		t.Skip("git not on PATH")
	}
	r := gitremote.New()
	_, ok := r.RemoteURL(t.TempDir())
	if ok {
		t.Error("ok should be false for non-repo dir")
	}
}

func TestResolver_RemoteURL_ReturnsConfiguredOrigin(t *testing.T) {
	if !gitOnPath(t) {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "--quiet")
	mustRun(t, dir, "git", "remote", "add", "origin", "git@example.com:foo/bar.git")

	r := gitremote.New()
	got, ok := r.RemoteURL(dir)
	if !ok {
		t.Fatal("ok should be true when remote is configured")
	}
	if got != "git@example.com:foo/bar.git" {
		t.Errorf("URL = %q", got)
	}
}

func TestResolver_RemoteURL_RepoWithoutOrigin_FalseOK(t *testing.T) {
	if !gitOnPath(t) {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "--quiet")

	r := gitremote.New()
	_, ok := r.RemoteURL(dir)
	if ok {
		t.Error("ok should be false when origin is missing")
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Suppress the default GIT_CONFIG_NOSYSTEM noise by isolating HOME.
	cmd.Env = append(
		os.Environ(),
		"HOME="+filepath.Join(dir, ".home"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
}
