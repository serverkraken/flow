package pgstore_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

// TestNodeLogoStore_PutIsOwnerScoped is the banner test's twin. node_id is the
// primary key, so a second owner's Put collides with the first owner's row;
// without an owner clause on the conflict update it overwrites another
// tenant's image while owner_id keeps naming the original owner.
func TestNodeLogoStore_PutIsOwnerScoped(t *testing.T) {
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
	owner, _ := domain.NewUser("u-logo-own", "sub-logo-own", "logoown", "logoown@x.de", "LogoOwn")
	if _, err := users.UpsertBySub(ctx, owner); err != nil {
		t.Fatal(err)
	}
	intruder, _ := domain.NewUser("u-logo-other", "sub-logo-other", "logoother", "logoother@x.de", "LogoOther")
	if _, err := users.UpsertBySub(ctx, intruder); err != nil {
		t.Fatal(err)
	}
	n, _ := domain.NewNode("eng-logo-own", owner.ID, "Engagement Logo", "eng-logo-own", time.Now().UTC())
	n.Kind = domain.KindEngagement
	if _, err := pgstore.NewNodeStore(pool).Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeLogoStore(pool)
	now := time.Now().UTC().Truncate(time.Millisecond)

	mine := domain.NodeLogo{
		NodeID: n.ID, OwnerID: owner.ID, Mime: "image/png",
		Ref: "0000aaaa0000", Bytes: []byte{1, 1, 1}, UpdatedAt: now, Width: 64, Height: 64,
	}
	if err := st.Put(ctx, mine); err != nil {
		t.Fatalf("put: %v", err)
	}

	theirs := domain.NodeLogo{
		NodeID: n.ID, OwnerID: intruder.ID, Mime: "image/png",
		Ref: "ffffbbbbffff", Bytes: []byte{7, 7, 7}, UpdatedAt: now.Add(time.Minute), Width: 8, Height: 8,
	}
	_ = st.Put(ctx, theirs) // must not reach the row; an error here is fine too

	after, err := st.Get(ctx, owner.ID, n.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Ref != "0000aaaa0000" || !bytes.Equal(after.Bytes, []byte{1, 1, 1}) {
		t.Errorf("a foreign Put overwrote the owner's logo: %+v", after)
	}
}
