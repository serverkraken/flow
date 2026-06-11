// internal/testutil/pgtest/pg.go
//
// Package pgtest starts a throwaway PostgreSQL container for store and
// handler tests. Mirrors the oidctest dex pattern, but fails loud instead
// of skipping: pgstore tests ARE the core test surface after R1, a silent
// skip would hollow out the coverage gate.
//
// Usage (one container per test package, shared store, isolation via
// per-test users — all tables are user-scoped):
//
//	var testStore *pgstore.Store   // package-level, set in TestMain
//
//	func TestMain(m *testing.M) { os.Exit(pgtest.RunWithStore(m, &testStore)) }
package pgtest

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

// StartContainer boots a postgres:16-alpine container and returns its DSN
// plus a terminate func. The caller owns termination.
func StartContainer(ctx context.Context) (dsn string, terminate func(), err error) {
	ctr, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("flow_test"),
		postgres.WithUsername("flow"),
		postgres.WithPassword("flow"),
		testcontainers.WithWaitStrategy(
			tcwait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("pgtest: start container (DOCKER_HOST gesetzt? podman machine läuft?): %w", err)
	}
	dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return "", nil, fmt.Errorf("pgtest: connection string: %w", err)
	}
	return dsn, func() { _ = ctr.Terminate(context.Background()) }, nil
}
