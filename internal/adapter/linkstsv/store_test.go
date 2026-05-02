package linkstsv_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/linkstsv"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.LinkStore = (*linkstsv.Store)(nil)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestListByDate_MissingFile(t *testing.T) {
	s := linkstsv.New(filepath.Join(t.TempDir(), "missing.tsv"))
	got, err := s.ListByDate(mustParseDate(t, "2026-04-30"))
	if err != nil {
		t.Fatalf("ListByDate: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestAdd_AndList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	d := mustParseDate(t, "2026-04-30")

	if err := s.Add(d, "note-a"); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if err := s.Add(d, "note-b"); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	got, err := s.ListByDate(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "note-a" || got[1] != "note-b" {
		t.Errorf("insertion order: got %v", got)
	}
}

func TestAdd_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	d := mustParseDate(t, "2026-04-30")

	if err := s.Add(d, "note-x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(d, "note-x"); err != nil {
		t.Fatalf("Add idempotent: %v", err)
	}

	got, _ := s.ListByDate(d)
	if len(got) != 1 {
		t.Errorf("want 1 entry, got %d", len(got))
	}

	raw, _ := os.ReadFile(path)
	if got := strings.Count(string(raw), "note-x"); got != 1 {
		t.Errorf("file should hold note-x once, holds %d times", got)
	}
}

func TestAdd_DifferentDates_SameNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	d1 := mustParseDate(t, "2026-04-30")
	d2 := mustParseDate(t, "2026-05-01")

	if err := s.Add(d1, "shared"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(d2, "shared"); err != nil {
		t.Fatal(err)
	}

	if got, _ := s.ListByDate(d1); len(got) != 1 || got[0] != "shared" {
		t.Errorf("d1: got %v", got)
	}
	if got, _ := s.ListByDate(d2); len(got) != 1 || got[0] != "shared" {
		t.Errorf("d2: got %v", got)
	}
}

func TestRemove_Existing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	d := mustParseDate(t, "2026-04-30")

	for _, id := range []string{"a", "b", "c"} {
		if err := s.Add(d, id); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Remove(d, "b"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListByDate(d)
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("after remove b: got %v", got)
	}
}

func TestRemove_Missing_NoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	d := mustParseDate(t, "2026-04-30")

	// Empty file.
	if err := s.Remove(d, "anything"); err != nil {
		t.Errorf("Remove on empty: %v", err)
	}

	// Populated file, missing ID.
	if err := s.Add(d, "real"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(d, "ghost"); err != nil {
		t.Errorf("Remove unknown id: %v", err)
	}

	got, _ := s.ListByDate(d)
	if len(got) != 1 || got[0] != "real" {
		t.Errorf("real entry should survive: got %v", got)
	}
}

func TestRead_TolerantParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	body := "" +
		"# header line\n" +
		"\n" +
		"   \n" +
		"2026-04-30\tgood\n" +
		"single-column\n" +
		"BOGUS\tbaddate\n" +
		"2026-04-30\t\n" + // empty noteID
		"2026-04-30\talso-good\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := linkstsv.New(path)
	got, err := s.ListByDate(mustParseDate(t, "2026-04-30"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "good" || got[1] != "also-good" {
		t.Errorf("got %v", got)
	}
}

func TestListByDate_FiltersOtherDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.tsv")
	s := linkstsv.New(path)
	for _, d := range []string{"2026-04-29", "2026-04-30", "2026-05-01"} {
		if err := s.Add(mustParseDate(t, d), "id-"+d); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.ListByDate(mustParseDate(t, "2026-04-30"))
	if len(got) != 1 || got[0] != "id-2026-04-30" {
		t.Errorf("got %v", got)
	}
}

func TestAdd_MkdirError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := linkstsv.New(filepath.Join(regular, "subdir", "links.tsv"))
	err := s.Add(mustParseDate(t, "2026-04-30"), "x")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestRemove_WriteAllFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.tsv")
	s := linkstsv.New(path)

	d := mustParseDate(t, "2026-04-30")
	if err := s.Add(d, "x"); err != nil {
		t.Fatal(err)
	}
	// Sabotage the rewrite: create a directory at the .tmp path so OpenFile fails.
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(d, "x"); err == nil {
		t.Error("want error, got nil")
	}
}

func TestList_OpenError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := linkstsv.New(filepath.Join(regular, "child"))
	_, err := s.ListByDate(mustParseDate(t, "2026-04-30"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("want non-NotExist error, got %v", err)
	}
}
