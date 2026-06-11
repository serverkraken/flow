// Package pgstore implements flow-server's persistence on PostgreSQL
// (pgx/v5 + goose PG migrations). It replaces internal/adapter/sqliteserver
// in the R1 server-only rebuild. Sub-adapters (Users, Projects, Sessions,
// ActiveSessions, Documents, DayOffs, Settings) share the pool via *Store.
//
// Versioning: per-row `version` incremented on every server-side write.
// The old global lamport counter is gone — without sync watermarks there
// is nothing that needs cross-row monotonicity.
package pgstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/serverkraken/flow/internal/adapter/pgstore/migrations"
)

// Store owns the pgx connection pool. Open runs pending goose migrations.
type Store struct{ pool *pgxpool.Pool }

// Open connects to dsn, pings, migrates, and returns a ready Store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgstore: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	// goose braucht *sql.DB; stdlib wrappt den Pool nur für die Migration.
	sqlDB := stdlib.OpenDBFromPool(pool)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: migrate: %w", err)
	}
	_ = sqlDB.Close()
	return &Store{pool: pool}, nil
}

// Pool exposes the pgx pool for the sub-adapters in this package.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping reports connectivity; wired into /readyz.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Close shuts the pool down.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}
