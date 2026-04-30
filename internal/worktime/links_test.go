package worktime_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestListLinks_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	ids, err := worktime.ListLinks(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("got %d ids, want 0", len(ids))
	}
}

func TestAddLink_PersistsAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	d := mustParseDate(t, "2026-04-28")

	if err := worktime.AddLink(d, "projects/foo/bar"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.AddLink(d, "projects/foo/bar"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.AddLink(d, "notes/zettel"); err != nil {
		t.Fatal(err)
	}

	ids, err := worktime.ListLinks(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2: %v", len(ids), ids)
	}
	if ids[0] != "projects/foo/bar" || ids[1] != "notes/zettel" {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestAddLink_DateScoping(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	day1 := mustParseDate(t, "2026-04-28")
	day2 := mustParseDate(t, "2026-04-29")

	if err := worktime.AddLink(day1, "daily/2026-04-28"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.AddLink(day2, "daily/2026-04-29"); err != nil {
		t.Fatal(err)
	}

	ids, _ := worktime.ListLinks(day1)
	if len(ids) != 1 || ids[0] != "daily/2026-04-28" {
		t.Errorf("day1 ids = %v", ids)
	}
	ids, _ = worktime.ListLinks(day2)
	if len(ids) != 1 || ids[0] != "daily/2026-04-29" {
		t.Errorf("day2 ids = %v", ids)
	}
}

func TestRemoveLink_RemovesOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	d := mustParseDate(t, "2026-04-28")

	for _, id := range []string{"a", "b", "c"} {
		if err := worktime.AddLink(d, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := worktime.RemoveLink(d, "b"); err != nil {
		t.Fatal(err)
	}
	ids, _ := worktime.ListLinks(d)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "c" {
		t.Errorf("after remove ids = %v, want [a c]", ids)
	}
}

func TestRemoveLink_MissingIsNoop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	d := mustParseDate(t, "2026-04-28")

	if err := worktime.RemoveLink(d, "ghost"); err != nil {
		t.Errorf("removing missing should be no-op, got: %v", err)
	}
}

func TestAddLink_RejectsInvalidIDs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	d := mustParseDate(t, "2026-04-28")

	cases := []string{"", "  ", "with\ttab", "with\nnewline"}
	for _, c := range cases {
		if err := worktime.AddLink(d, c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestListLinks_SkipsMalformedRows(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Pre-seed file with garbage + valid row.
	tmuxDir := filepath.Join(dir, ".tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "" +
		"# comment\n" +
		"\n" +
		"not-a-date\tnote/x\n" +
		"2026-04-28\t\n" +
		"2026-04-28\tdaily/2026-04-28\n" +
		"only-one-column\n"
	if err := os.WriteFile(filepath.Join(tmuxDir, "worktime-links.tsv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	d := mustParseDate(t, "2026-04-28")
	ids, err := worktime.ListLinks(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "daily/2026-04-28" {
		t.Errorf("ids = %v, want [daily/2026-04-28]", ids)
	}
}
