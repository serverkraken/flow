package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestSearch_TableOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	note := mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "About kompendium", "kompendium architecture body")
	_ = env.index.Upsert(context.Background(), note, time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "search", "kompendium")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("expected hit in output, got %q", stdout)
	}
}

func TestSearch_JSONOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	note := mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "title", "alpha body")
	_ = env.index.Upsert(context.Background(), note, time.Unix(1, 0))

	stdout, _, err := runCmd(t, env.deps, "search", "alpha", "--json")
	if err != nil {
		t.Fatalf("search --json: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, stdout)
	}
	if len(rows) != 1 || rows[0]["id"] != "daily/2026-04-25" {
		t.Errorf("rows got %+v", rows)
	}
}

func TestSearch_OrderRecent(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_ = env.index.Upsert(context.Background(),
		mustNote(t, "daily/2026-04-22", domain.TypeDaily, "", "", "shared"),
		time.Unix(1, 0),
	)
	_ = env.index.Upsert(context.Background(),
		mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", "shared"),
		time.Unix(2, 0),
	)

	if _, _, err := runCmd(t, env.deps, "search", "shared", "--order", "recent"); err != nil {
		t.Fatalf("search recent: %v", err)
	}
}

func TestSearch_BadOrder(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "search", "x", "--order", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid --order")
	}
}
