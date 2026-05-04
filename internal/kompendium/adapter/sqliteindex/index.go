package sqliteindex

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	// Pure-Go SQLite driver — registers under the "sqlite" name.
	_ "modernc.org/sqlite"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ErrIndexClosed is returned by Search/BacklinksOf/LinksFrom and the
// CRUD methods after Close has been called. Without this guard a
// caller racing against shutdown would observe a panic from the
// underlying sql.DB rather than a typed error.
var ErrIndexClosed = errors.New("kompendium index closed")

// schemaVersion is bumped whenever the on-disk shape changes (new
// table/column, altered FTS5 tokenizer, etc.). New() refuses to open a
// database whose user_version is *higher* than this constant — that
// means the file was written by a newer flow build and the current
// binary cannot safely query it. A *lower* user_version is migrated
// in-place (currently only `0 → 1`, which is the legacy "no version"
// state from before this guard existed and shares the same shape).
const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS notes (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    project TEXT,
    date TEXT,
    title TEXT,
    mtime INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS links (
    src_id TEXT NOT NULL,
    dst_id TEXT NOT NULL,
    PRIMARY KEY (src_id, dst_id)
);
CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
    id UNINDEXED,
    title,
    body,
    tokenize = "unicode61 remove_diacritics 1"
);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type);
CREATE INDEX IF NOT EXISTS idx_notes_project ON notes(project);
CREATE INDEX IF NOT EXISTS idx_links_dst ON links(dst_id);
`

// ErrSchemaTooNew signals that the on-disk index was written by a
// newer flow binary and the current build cannot safely use it. The
// CLI prints this with a hint to upgrade or rebuild the index.
var ErrSchemaTooNew = errors.New("kompendium index schema is newer than this build supports")

// Indexer implements ports.Indexer using a SQLite + FTS5 database.
//
// mu serialises Close against in-flight queries: the browse TUI runs
// Search in a goroutine via tea.Cmd, and main()'s defer cleanup() can
// fire while one of those queries is still scanning rows. Without the
// rwMutex the underlying sql.DB closes mid-scan and panics inside the
// driver. Search/BacklinksOf/LinksFrom (and the CRUD path) acquire RLock;
// Close acquires Lock and clears db so subsequent calls error cleanly.
type Indexer struct {
	mu     sync.RWMutex
	db     *sql.DB
	closed bool
}

// New opens a SQLite database at dbPath (use ":memory:" for tests) and
// initialises the schema. The caller owns the *Indexer and must Close it.
//
// File-backed databases are tuned for kompendium's workload:
//   - journal_mode=WAL: readers (browse TUI) don't block the writer (CLI
//     `new daily` from another tmux pane), and writes commit faster.
//   - busy_timeout=5000: on contention, sqlite waits up to 5s instead of
//     immediately returning SQLITE_BUSY.
//   - synchronous=NORMAL: paired with WAL, durability without an fsync per
//     transaction. Acceptable for an index that is always rebuildable from
//     the notebook.
//
// :memory: ignores journal_mode anyway and benefits from neither pragma,
// so the tuning is skipped to keep tests deterministic.
func New(dbPath string) (*Indexer, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = dbPath + "?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=synchronous(normal)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %q: %w", dbPath, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := ensureSchemaVersion(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Indexer{db: db}, nil
}

// ensureSchemaVersion compares the on-disk PRAGMA user_version against
// the binary's schemaVersion. New databases (user_version=0) get
// stamped with the current version. Older versions get migrated (today
// 0 and 1 share the same shape, so the migration is just a stamp).
// Newer versions return ErrSchemaTooNew so the binary fails fast
// instead of running queries against an unknown shape.
func ensureSchemaVersion(db *sql.DB) error {
	var have int
	if err := db.QueryRow("PRAGMA user_version").Scan(&have); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if have > schemaVersion {
		return fmt.Errorf("%w: file is at version %d, binary supports up to %d (run `flow kompendium index rebuild` after upgrading)", ErrSchemaTooNew, have, schemaVersion)
	}
	if have == schemaVersion {
		return nil
	}
	// PRAGMA user_version takes a literal int — sql parameter bindings
	// are not honoured for it. Inlining schemaVersion is safe because
	// the constant is compile-time fixed.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("stamp user_version: %w", err)
	}
	return nil
}

// Close releases the database handle. Blocks until in-flight queries
// (Search, BacklinksOf, LinksFrom, CRUD) finish. Idempotent: a second
// Close after the first reports nil.
func (i *Indexer) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	return i.db.Close()
}

// guard enters a read-side critical section against Close. The returned
// release closure is intended to be deferred by the caller. Callers MUST
// check the error before using i.db; on ErrIndexClosed they must return
// without dereferencing the handle.
func (i *Indexer) guard() (release func(), err error) {
	i.mu.RLock()
	if i.closed {
		i.mu.RUnlock()
		return func() {}, ErrIndexClosed
	}
	return i.mu.RUnlock, nil
}

var _ ports.Indexer = (*Indexer)(nil)
