package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestDoctor_UncommittedChanges_RendersHint(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true
	env.git.HasChangesValue = true
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "Daily", ""), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "uncommitted") {
		t.Errorf("expected uncommitted-changes hint in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "kompendium snapshot") {
		t.Errorf("expected snapshot suggestion, got:\n%s", stdout)
	}
}

func TestDoctor_ExitNonZeroOnIssues(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.git.IsRepoValue = true
	// Seed a note with a broken wikilink so the report is not clean.
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "",
		"Daily", "see [[notes/missing]]"), time.Unix(1, 0))

	_, _, err := runCmd(t, env.deps, "doctor", "--exit-non-zero-on-issues")
	if err == nil {
		t.Errorf("expected non-zero exit when issues exist with --exit-non-zero-on-issues")
	}
}
