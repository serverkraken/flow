package pgstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// One shared postgres container per `go test` run, one fresh database per
// test. The previous per-test containers churned ~100 postgres starts/stops
// through the CI runner's docker daemon; under that load a random subset of
// tests died with "connection reset by peer".
var (
	pgOnce      sync.Once
	pgStartErr  error
	pgContainer *tcpg.PostgresContainer
	pgAdminDSN  string
	pgAdmin     *pgxpool.Pool
	pgDBSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	// Ryuk (the reaper container) requires a bridge network that Podman does not
	// expose by default. Disable it so tests pass on Podman-based CI and local
	// dev environments. Cleanup happens below instead.
	_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	code := m.Run()
	if pgAdmin != nil {
		pgAdmin.Close()
	}
	if pgContainer != nil {
		_ = pgContainer.Terminate(context.Background())
	}
	os.Exit(code)
}

func startPG(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	pgOnce.Do(func() {
		// The postgres image starts the server TWICE (initdb bootstrap, then the
		// real server). ForListeningPort unblocks during the bootstrap server,
		// so connecting right after races the restart and dies with "connection
		// reset by peer" on slow runners. Wait for the second "ready to accept
		// connections" log line instead — the strategy the testcontainers
		// postgres module also uses by default.
		c, err := tcpg.Run(ctx, "pgvector/pgvector:pg16",
			tcpg.WithDatabase("flow_test"), tcpg.WithUsername("flow"), tcpg.WithPassword("flow"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(120*time.Second)))
		if err != nil {
			pgStartErr = err
			return
		}
		pgContainer = c
		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgStartErr = err
			return
		}
		pgAdminDSN = dsn
		pgAdmin, pgStartErr = pgxpool.New(ctx, dsn)
	})
	if pgStartErr != nil {
		t.Skipf("docker unavailable: %v", pgStartErr)
	}
	// Fresh database per test; safe under t.Parallel because postgres
	// serializes CREATE DATABASE and the name is unique per counter.
	name := fmt.Sprintf("flow_test_%d", pgDBSeq.Add(1))
	if _, err := pgAdmin.Exec(ctx, "CREATE DATABASE "+name+" OWNER flow"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		// FORCE kills stragglers; the test's own pool.Close cleanup runs first
		// (LIFO), so this is just a safety net.
		_, _ = pgAdmin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return strings.Replace(pgAdminDSN, "/flow_test?", "/"+name+"?", 1)
}

func TestUserStoreUpsertGet(t *testing.T) {
	ctx := context.Background()
	dsn := startPG(t)
	pool, err := pgstore.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewUserStore(pool)

	if _, err := store.GetBySub(ctx, "nope"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := store.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	// upsert again with changed profile keeps id, updates fields
	u2, _ := domain.NewUser("u-other", "sub-1", "msoent", "new@x.de", "Martin S")
	got, err := store.UpsertBySub(ctx, u2)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "u1" {
		t.Fatalf("upsert must keep original id, got %q", got.ID)
	}
	if got.Email != "new@x.de" {
		t.Fatalf("email not updated: %q", got.Email)
	}
}
