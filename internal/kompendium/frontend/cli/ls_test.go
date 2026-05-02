package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestLs_TableOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily title", ""), time.Unix(2, 0))
	env.store.Seed(mustNote(t, "notes/setup", domain.TypeFree, "", "Setup notes", ""), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "ls")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("daily missing in output %q", stdout)
	}
	if !strings.Contains(stdout, "notes/setup") {
		t.Errorf("free missing in output %q", stdout)
	}
}

func TestLs_TypeFilter(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily", ""), time.Unix(2, 0))
	env.store.Seed(mustNote(t, "notes/setup", domain.TypeFree, "", "Setup", ""), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "ls", "--type", "daily")
	if err != nil {
		t.Fatalf("ls --type=daily: %v", err)
	}
	if strings.Contains(stdout, "notes/setup") {
		t.Errorf("type filter leaked free note: %q", stdout)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("daily missing: %q", stdout)
	}
}

func TestLs_JSONOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily", ""), time.Unix(2, 0))

	stdout, _, err := runCmd(t, env.deps, "ls", "--json")
	if err != nil {
		t.Fatalf("ls --json: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout)
	}
	if len(rows) != 1 || rows[0]["id"] != "daily/2026-04-25" {
		t.Errorf("rows got %+v", rows)
	}
}

func TestLs_CurrentRepoPromotes(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", ""), time.Unix(3, 0))
	env.store.Seed(mustNote(t, "projects/github.com/foo/bar/2026-04-22", domain.TypeProject, "github.com/foo/bar", "", ""), time.Unix(1, 0))
	env.store.Seed(mustNote(t, "projects/github.com/foo/bar/2026-04-25", domain.TypeProject, "github.com/foo/bar", "", ""), time.Unix(2, 0))

	stdout, _, err := runCmd(t, env.deps, "ls", "--current-repo", "github.com/foo/bar")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >=2 lines, got %d (%q)", len(lines), stdout)
	}
	// Top tier is the current-repo project notes; daily falls to tier 1.
	if !strings.HasPrefix(lines[0], "projects/github.com/foo/bar") {
		t.Errorf("expected current-repo project first, got %q", lines[0])
	}
}
