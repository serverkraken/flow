package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestFeedTokenStore_CreateResolveRevoke(t *testing.T) {
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

	store := pgstore.NewFeedTokenStore(pool)
	ft := domain.FeedToken{Token: "tok-abc", UserID: "u1", Kind: "ics", CreatedAt: time.Now()}
	if err := store.Create(ctx, ft); err != nil {
		t.Fatalf("create: %v", err)
	}
	owner, err := store.Resolve(ctx, "tok-abc")
	if err != nil || owner != "u1" {
		t.Fatalf("resolve = %q err=%v", owner, err)
	}
	if _, err := store.Resolve(ctx, "nope"); !errors.Is(err, ports.ErrFeedTokenNotFound) {
		t.Fatalf("unknown token: want ErrFeedTokenNotFound, got %v", err)
	}
	if err := store.Revoke(ctx, "u1", "tok-abc"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.Resolve(ctx, "tok-abc"); !errors.Is(err, ports.ErrFeedTokenNotFound) {
		t.Fatalf("revoked token must not resolve, got %v", err)
	}
}
