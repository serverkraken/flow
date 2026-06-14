package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestDayOffStore_AddListDelete(t *testing.T) {
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

	store := pgstore.NewDayOffStore(pool)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := store.Add(ctx, "u1", domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Re-add same day = upsert (no unique violation), updates kind/label/target.
	if err := store.Add(ctx, "u1", domain.DayOff{Date: day, Kind: domain.KindSick, Label: "", Target: 4 * time.Hour}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.ListRange(ctx, "u1", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1))
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %d err=%v", len(got), err)
	}
	if got[0].Kind != domain.KindSick || got[0].Target != 4*time.Hour {
		t.Fatalf("upsert did not overwrite: %+v", got[0])
	}
	// Owner isolation.
	if other, _ := store.ListRange(ctx, "u2", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)); len(other) != 0 {
		t.Fatalf("owner isolation broken: %d", len(other))
	}
	// Delete is idempotent.
	if err := store.Delete(ctx, "u1", day); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, "u1", day); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
	if got, _ := store.ListRange(ctx, "u1", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)); len(got) != 0 {
		t.Fatalf("want empty after delete, got %d", len(got))
	}
}
