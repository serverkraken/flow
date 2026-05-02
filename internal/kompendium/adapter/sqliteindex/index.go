package sqliteindex

import (
	"database/sql"
	"fmt"

	// Pure-Go SQLite driver — registers under the "sqlite" name.
	_ "modernc.org/sqlite"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

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

// Indexer implements ports.Indexer using a SQLite + FTS5 database.
type Indexer struct {
	db *sql.DB
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
	return &Indexer{db: db}, nil
}

// Close releases the database handle.
func (i *Indexer) Close() error { return i.db.Close() }

var _ ports.Indexer = (*Indexer)(nil)
