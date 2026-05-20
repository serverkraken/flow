package cli

// Direct tests for the small helper functions in browse.go that the
// black-box runBrowse-swapping tests don't reach:
//   - backlinksFromUsecase: closure-returning factory
//   - indexAgeFromFile:     os.Stat-based time accessor
//   - BuildWriteCmd:        kompendium new <type> argv builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestBacklinksFromUsecase_ErrorReturnsNil(t *testing.T) {
	t.Parallel()
	ix := testutil.NewFakeIndexer()
	ix.BacklinksErr = errFake("oops")
	store := testutil.NewFakeNoteStore()
	uc := usecase.NewRenderBacklinks(store, ix)
	fn := backlinksFromUsecase(context.Background(), uc)
	if got := fn(domain.ID("notes/x")); got != nil {
		t.Errorf("error path should yield nil, got %+v", got)
	}
}

func TestBacklinksFromUsecase_HappyPath(t *testing.T) {
	t.Parallel()
	ix := testutil.NewFakeIndexer()
	store := testutil.NewFakeNoteStore()
	// Seed an A→B link via the indexer.
	a := mustNoteFor(t, "daily/2026-05-01", "A", domain.TypeDaily, "[[notes/b]]")
	b := mustNoteFor(t, "notes/b", "B", domain.TypeFree, "")
	now := time.Now()
	if err := ix.Upsert(context.Background(), a, now); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := ix.Upsert(context.Background(), b, now); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	// Seed the store too so RenderBacklinks's Lookup succeeds.
	store.Seed(b, now)
	uc := usecase.NewRenderBacklinks(store, ix)
	fn := backlinksFromUsecase(context.Background(), uc)
	got := fn(b.ID)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("backlinks fn(b): got %+v, want [%s]", got, a.ID)
	}
}

func TestIndexAgeFromFile_MissingReturnsZero(t *testing.T) {
	t.Parallel()
	fn := indexAgeFromFile(filepath.Join(t.TempDir(), "no-such.db"))
	if got := fn(); !got.IsZero() {
		t.Errorf("missing index file should yield zero time, got %s", got)
	}
}

func TestIndexAgeFromFile_ExistingReturnsModTime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "index.db")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fn := indexAgeFromFile(p)
	if got := fn(); got.IsZero() {
		t.Errorf("existing file should yield non-zero mtime")
	}
}

func TestBuildWriteCmd_Daily(t *testing.T) {
	t.Parallel()
	fn := BuildWriteCmd("/repos/x")
	cmd := fn(writepicker.Result{Choice: writepicker.ChoiceDaily})
	if cmd == nil {
		t.Fatal("Daily should produce a command")
	}
	if len(cmd.Args) < 3 || cmd.Args[len(cmd.Args)-2] != "new" || cmd.Args[len(cmd.Args)-1] != "daily" {
		t.Errorf("Daily argv: %v", cmd.Args)
	}
}

func TestBuildWriteCmd_Project(t *testing.T) {
	t.Parallel()
	fn := BuildWriteCmd("/repos/y")
	cmd := fn(writepicker.Result{Choice: writepicker.ChoiceProject})
	if cmd == nil {
		t.Fatal("Project should produce a command")
	}
	hasCwd := false
	for i, a := range cmd.Args {
		if a == "--cwd" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "/repos/y" {
			hasCwd = true
		}
	}
	if !hasCwd {
		t.Errorf("Project argv should carry --cwd /repos/y, got %v", cmd.Args)
	}
}

func TestBuildWriteCmd_Free(t *testing.T) {
	t.Parallel()
	fn := BuildWriteCmd("/repos/z")
	cmd := fn(writepicker.Result{Choice: writepicker.ChoiceFree, Slug: "my-note"})
	if cmd == nil {
		t.Fatal("Free should produce a command")
	}
	if cmd.Args[len(cmd.Args)-1] != "my-note" {
		t.Errorf("Free argv last arg should be slug, got %v", cmd.Args)
	}
}

func TestBuildWriteCmd_UnknownChoice(t *testing.T) {
	t.Parallel()
	fn := BuildWriteCmd("/repos/q")
	if cmd := fn(writepicker.Result{Choice: -1}); cmd != nil {
		t.Errorf("unknown choice should yield nil cmd, got %+v", cmd)
	}
}

// helpers

type errStr string

func (e errStr) Error() string { return string(e) }

func errFake(s string) error { return errStr(s) }

func mustNoteFor(t *testing.T, id, title string, typ domain.NoteType, body string) domain.Note {
	t.Helper()
	fm := domain.Frontmatter{ID: id, Type: typ, Title: title}
	if typ == domain.TypeProject {
		fm.Project = "github.com/foo/bar"
	}
	n, err := domain.NewNote(domain.ID(id), fm, []byte(body))
	if err != nil {
		t.Fatalf("NewNote(%s): %v", id, err)
	}
	return n
}
