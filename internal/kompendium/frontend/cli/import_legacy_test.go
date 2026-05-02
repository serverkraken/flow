package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestImportLegacy_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.legacy.Dailies = []ports.LegacyDaily{
		{Path: "/n/2026-04-25.md", Date: "2026-04-25", Body: []byte("daily body\n")},
	}
	env.legacy.Projects = []ports.LegacyProject{
		{Path: "/pn/x.md", URL: "git@github.com:foo/bar.git", Body: []byte("body\n")},
	}

	stdout, _, err := runCmd(t, env.deps, "import-legacy", "--daily-dir", "/x", "--project-dir", "/y")
	if err != nil {
		t.Fatalf("import-legacy: %v", err)
	}
	if !strings.Contains(stdout, "Migrated: 2") {
		t.Errorf("expected migrated count in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("expected daily ID in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "projects/github.com/foo/bar/_project") {
		t.Errorf("expected canonical project ID in output, got %q", stdout)
	}
}

func TestImportLegacy_ReportsSkipped(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.legacy.Projects = []ports.LegacyProject{
		{Path: "/pn/no-remote.md", URL: "", Body: []byte("body\n")},
	}

	stdout, _, err := runCmd(t, env.deps, "import-legacy", "--daily-dir", "/x", "--project-dir", "/y")
	if err != nil {
		t.Fatalf("import-legacy: %v", err)
	}
	if !strings.Contains(stdout, "Skipped: 1") {
		t.Errorf("missing skipped section in %q", stdout)
	}
	if !strings.Contains(stdout, "no Remote: URL extracted") {
		t.Errorf("missing skip reason in %q", stdout)
	}
}

func TestImportLegacy_AdapterError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced daily list err")
	env.legacy.DailyErr = forced

	_, _, err := runCmd(t, env.deps, "import-legacy", "--daily-dir", "/x", "--project-dir", "/y")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestImportLegacy_DefaultDirs_NotesDirEnv(t *testing.T) {
	// t.Setenv blocks t.Parallel.
	env := newTestEnv(t)
	t.Setenv("NOTES_DIR", "/some/notes-dir")
	t.Setenv("HOME", t.TempDir())

	if _, _, err := runCmd(t, env.deps, "import-legacy"); err != nil {
		t.Fatal(err)
	}
}

func TestImportLegacy_DefaultDirs_HomeFallback(t *testing.T) {
	env := newTestEnv(t)
	t.Setenv("NOTES_DIR", "")
	t.Setenv("HOME", t.TempDir())

	if _, _, err := runCmd(t, env.deps, "import-legacy"); err != nil {
		t.Fatal(err)
	}
}
