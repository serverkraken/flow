package pgstore_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestUserSettingsStore_LazyDefaultAndSet(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	store := pgstore.NewUserSettingsStore(pool)
	// No row yet → lazy default NW.
	got, err := store.Get(ctx, "u1")
	if err != nil || got.Bundesland != "NW" {
		t.Fatalf("lazy default = %+v err=%v", got, err)
	}
	if err := store.SetBundesland(ctx, "u1", "BY"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ = store.Get(ctx, "u1")
	if got.Bundesland != "BY" {
		t.Fatalf("want BY after set, got %q", got.Bundesland)
	}
}
