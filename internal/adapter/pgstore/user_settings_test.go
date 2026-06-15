package pgstore_test

import (
	"context"
	"testing"
	"time"

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

func TestUserSettings_TargetConfigRoundTrip(t *testing.T) {
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

	st := pgstore.NewUserSettingsStore(pool)

	got, err := st.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTargetMin != domain.DefaultDailyTargetMin || len(got.WeekdayTargetMin) != 0 {
		t.Fatalf("lazy default: got default=%d weekday=%v", got.DefaultTargetMin, got.WeekdayTargetMin)
	}

	if err := st.SetTargetConfig(ctx, "u1", 420, map[time.Weekday]int{time.Friday: 360, time.Saturday: 0}); err != nil {
		t.Fatal(err)
	}
	got, err = st.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTargetMin != 420 {
		t.Errorf("default: got %d want 420", got.DefaultTargetMin)
	}
	if v, ok := got.WeekdayTargetMin[time.Friday]; !ok || v != 360 {
		t.Errorf("friday override: got %d ok=%v want 360", v, ok)
	}
	if v, ok := got.WeekdayTargetMin[time.Saturday]; !ok || v != 0 {
		t.Errorf("saturday explicit-0 override lost: got %d ok=%v", v, ok)
	}
	if _, ok := got.WeekdayTargetMin[time.Monday]; ok {
		t.Errorf("monday should have no override")
	}

	if err := st.SetBundesland(ctx, "u1", "BY"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTargetConfig(ctx, "u1", domain.DefaultDailyTargetMin, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(ctx, "u1")
	if got.Bundesland != "BY" {
		t.Errorf("bundesland clobbered: got %q", got.Bundesland)
	}
}
