package sqliteclient

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" sql driver

	"github.com/serverkraken/flow/internal/adapter/sqliteclient/migrations"
)

// Store owns the *sql.DB and exposes raw access to sub-adapters in this
// package. Construction enables foreign-keys and runs migrations.
type Store struct {
	db *sql.DB
}

// Open opens or creates the SQLite database at path, enables relevant
// pragmas, and applies pending migrations via embedded goose. Caller must
// Close().
//
// Migrations are idempotent — re-opening an existing DB is safe.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient: ping: %w", err)
	}

	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.FS)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient: provider: %w", err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient: migrate: %w", err)
	}
	// SQLite only supports one concurrent writer. Serialise all writes through
	// a single connection so the driver never races for the write lock.
	db.SetMaxOpenConns(1)

	return &Store{db: db}, nil
}

// DB returns the raw *sql.DB. Sub-adapters in this package use it
// directly; callers outside the package should not.
func (s *Store) DB() *sql.DB { return s.db }

// Close shuts the connection down. Safe to call multiple times.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
