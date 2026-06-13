package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearch_TableOutput(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	// Seed a document into the fake doc store so SearchNotes.Execute finds it.
	if _, err := env.docs.Put("testuser", "daily/2026-04-25.md", "kompendium architecture body", "", 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

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
	if _, err := env.docs.Put("testuser", "daily/2026-04-25.md", "alpha body", "", 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

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
	if _, err := env.docs.Put("testuser", "daily/2026-04-22.md", "shared", "", 0); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := env.docs.Put("testuser", "daily/2026-04-25.md", "shared", "", 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

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
