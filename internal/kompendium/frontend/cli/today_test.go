package cli_test

import (
	"strings"
	"testing"
)

func TestToday_AliasForNewDaily(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	stdout, _, err := runCmd(t, env.deps, "today")
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("expected today's daily ID in output, got %q", stdout)
	}
	// `today` shares the create-daily code path, so the editor was invoked
	// once on the resolved note path.
	if len(env.editor.Calls) != 1 {
		t.Errorf("editor calls got %d, want 1", len(env.editor.Calls))
	}
}

func TestToday_HelpListsTheSubcommand(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "today", "--help")
	if err != nil {
		t.Fatalf("today --help: %v", err)
	}
	if !strings.Contains(stdout, "same code path") && !strings.Contains(stdout, "new daily") {
		t.Errorf("help text should reference the new-daily code path, got %q", stdout)
	}
}
