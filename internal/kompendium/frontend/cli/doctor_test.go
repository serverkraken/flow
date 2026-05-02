package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestDoctor_CleanReport(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily", ""), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "All checks passed") {
		t.Errorf("expected clean confirmation, got %q", stdout)
	}
	if !strings.Contains(stdout, "repo (clean)") {
		t.Errorf("expected git status line, got %q", stdout)
	}
}

func TestDoctor_NotARepoHint(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = false

	stdout, _, err := runCmd(t, env.deps, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "kompendium init") {
		t.Errorf("missing init hint in output %q", stdout)
	}
}

func TestDoctor_BrokenLinksReported(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily",
		"see [[notes/missing]] and [[notes/setup]]"), time.Unix(1, 0))
	env.store.Seed(mustNote(t, "notes/setup", domain.TypeFree, "", "Setup", ""), time.Unix(2, 0))

	stdout, _, err := runCmd(t, env.deps, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "Broken wikilinks") {
		t.Errorf("missing broken-links section in %q", stdout)
	}
	if !strings.Contains(stdout, "notes/missing") {
		t.Errorf("missing target id in output %q", stdout)
	}
	if strings.Contains(stdout, "All checks passed") {
		t.Error("must not report all-passed when there are issues")
	}
}

func TestDoctor_JSONOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true
	env.git.HasChangesValue = true
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", ""), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout)
	}
	if !report["IsRepo"].(bool) {
		t.Errorf("IsRepo got %v", report["IsRepo"])
	}
	if !report["HasUncommitted"].(bool) {
		t.Errorf("HasUncommitted got %v", report["HasUncommitted"])
	}
}
