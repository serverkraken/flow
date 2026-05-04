package sqliteindex_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	// Pure-Go SQLite driver — registers under the "sqlite" name.
	_ "modernc.org/sqlite"

	"github.com/serverkraken/flow/internal/kompendium/adapter/sqliteindex"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestNew_InMemory(t *testing.T) {
	t.Parallel()
	idx, err := sqliteindex.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
}

func TestNew_FilePathPersists(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "index.db")

	idx1, err := sqliteindex.New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if err := idx1.Upsert(context.Background(), makeNote(t, "daily/2026-04-25", "hello body"), unix(1)); err != nil {
		t.Fatal(err)
	}
	if err := idx1.Close(); err != nil {
		t.Fatal(err)
	}

	idx2, err := sqliteindex.New(dbPath)
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	t.Cleanup(func() { _ = idx2.Close() })

	got, err := idx2.Search(context.Background(), domain.SearchQuery{Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 result after reopen, got %d", len(got))
	}
}

func TestNew_BadPath(t *testing.T) {
	t.Parallel()
	_, err := sqliteindex.New("/this-directory-must-not-exist/db.sqlite")
	if err == nil {
		t.Error("expected error opening DB under non-existent directory")
	}
}

// TestNew_WALModeOnDisk verifies the sqlite tuning lands: WAL mode creates
// `<dbpath>-wal` + `<dbpath>-shm` sidecar files as soon as a write
// transaction commits. Catch a regression where the DSN pragma silently
// drops back to journal_mode=delete.
func TestNew_WALModeOnDisk(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	idx, err := sqliteindex.New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.Upsert(context.Background(),
		makeNote(t, "daily/2026-04-25", "wal probe"), unix(1)); err != nil {
		t.Fatal(err)
	}

	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); err != nil {
		t.Errorf("expected %q (WAL sidecar), got stat err: %v", walPath, err)
	}
}

func TestErrorsAfterClose(t *testing.T) {
	t.Parallel()
	idx, err := sqliteindex.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := idx.Upsert(ctx, makeNote(t, "daily/x", "y"), unix(1)); err == nil {
		t.Error("expected error from Upsert after Close")
	}
	if err := idx.Delete(ctx, "daily/x"); err == nil {
		t.Error("expected error from Delete after Close")
	}
	if _, err := idx.Search(ctx, domain.SearchQuery{Text: "y"}); err == nil {
		t.Error("expected error from Search after Close")
	}
	if _, err := idx.BacklinksOf(ctx, "daily/x"); err == nil {
		t.Error("expected error from BacklinksOf after Close")
	}
	if _, err := idx.LinksFrom(ctx, "daily/x"); err == nil {
		t.Error("expected error from LinksFrom after Close")
	}
	if err := idx.Rebuild(ctx, nil); err == nil {
		t.Error("expected error from Rebuild after Close")
	}
}

// TestNew_StampsSchemaVersion verifies that opening a fresh DB writes
// the current PRAGMA user_version, so future builds can recognise it.
func TestNew_StampsSchemaVersion(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "v.db")
	idx, err := sqliteindex.New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v == 0 {
		t.Errorf("expected user_version > 0 after first open, got 0")
	}
}

// TestNew_RejectsFutureSchema verifies that a DB stamped with a
// version higher than the binary supports is refused with
// ErrSchemaTooNew rather than silently producing wrong results.
func TestNew_RejectsFutureSchema(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "future.db")
	// First open + close to install the schema.
	idx, err := sqliteindex.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	// Bump user_version to a value the current build cannot know.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 9999"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteindex.New(dbPath); !errors.Is(err, sqliteindex.ErrSchemaTooNew) {
		t.Errorf("expected ErrSchemaTooNew, got %v", err)
	}
}

// --- helpers shared with crud_test.go / query_test.go --------------------

func newIdx(t *testing.T) *sqliteindex.Indexer {
	t.Helper()
	idx, err := sqliteindex.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func makeNote(t *testing.T, idStr, body string) domain.Note {
	t.Helper()
	return makeNoteWithType(t, idStr, domain.TypeDaily, "", body)
}

func makeNoteAtID(t *testing.T, id domain.ID, body string) domain.Note {
	t.Helper()
	return makeNoteWithType(t, id.String(), domain.TypeDaily, "", body)
}

func makeNoteWithType(t *testing.T, idStr string, typ domain.NoteType, project, body string) domain.Note {
	t.Helper()
	id, err := domain.ParseID(idStr)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", idStr, err)
	}
	fm := domain.Frontmatter{
		ID:      id.String(),
		Type:    typ,
		Project: project,
		Title:   "title for " + idStr,
	}
	n, err := domain.NewNote(id, fm, []byte(body))
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}

func unix(n int64) time.Time { return time.Unix(n, 0) }
