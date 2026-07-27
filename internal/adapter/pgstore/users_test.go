package pgstore_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) {
	// Ryuk (the reaper container) requires a bridge network that Podman does not
	// expose by default. Disable it so tests pass on Podman-based CI and local
	// dev environments. Container cleanup is handled via t.Cleanup instead.
	_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	os.Exit(m.Run())
}

func startPG(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcpg.Run(ctx, "pgvector/pgvector:pg16",
		tcpg.WithDatabase("flow_test"), tcpg.WithUsername("flow"), tcpg.WithPassword("flow"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return dsn
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
