package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestDailyRender_WithProjects(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", "# Daily body\n"), time.Unix(10, 0))
	env.store.Seed(mustNote(t, "projects/foo/2026-04-25", domain.TypeProject, "github.com/foo", "Foo work", ""), time.Unix(11, 0))
	env.store.Seed(mustNote(t, "projects/bar/2026-04-25", domain.TypeProject, "github.com/bar", "Bar fixes", ""), time.Unix(12, 0))
	// The aggregation requires Date matching daily.Date; the helper sets
	// Date="2026-04-25" only for daily notes, so we patch projects manually.
	for _, id := range []domain.ID{"projects/foo/2026-04-25", "projects/bar/2026-04-25"} {
		n, _ := env.store.Get(t.Context(), id)
		n.Meta.Date = "2026-04-25"
		_ = env.store.Put(t.Context(), n)
	}

	stdout, _, err := runCmd(t, env.deps, "daily-render", "daily/2026-04-25")
	if err != nil {
		t.Fatalf("daily-render: %v", err)
	}
	if !strings.Contains(stdout, "## Projekt-Notizen heute") {
		t.Errorf("missing aggregation header in %q", stdout)
	}
	if !strings.Contains(stdout, "[[projects/foo/2026-04-25]]") {
		t.Errorf("missing project wikilink in %q", stdout)
	}
	if !strings.Contains(stdout, "# Daily body") {
		t.Errorf("missing daily body in %q", stdout)
	}
}

func TestDailyRender_NoProjects(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", "# Just the daily\n"), time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "daily-render", "daily/2026-04-25")
	if err != nil {
		t.Fatalf("daily-render: %v", err)
	}
	if strings.Contains(stdout, "## Projekt-Notizen") {
		t.Errorf("aggregation header should be absent when no projects, got %q", stdout)
	}
	if !strings.Contains(stdout, "# Just the daily") {
		t.Errorf("daily body missing: %q", stdout)
	}
}

func TestDailyRender_BadID(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "daily-render", "../escape")
	if err == nil {
		t.Fatal("expected error for bad ID")
	}
}
