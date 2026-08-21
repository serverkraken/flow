package pgstore_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TestNodeBannerStore_RoundTripAndOwnerScope covers the banner blob store:
// upsert-replaces, owner-scoped reads and deletes, and the absent case. The
// foreign-owner reads are the ones that matter — a banner is user content and
// must never be reachable across tenants.
func TestNodeBannerStore_RoundTripAndOwnerScope(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-bn", "sub-bn", "bn", "bn@x.de", "BN")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	n, _ := domain.NewNode("eng-bn", u.ID, "Engagement BN", "eng-bn", time.Now().UTC())
	n.Kind = domain.KindEngagement
	if _, err := pgstore.NewNodeStore(pool).Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeBannerStore(pool)
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := st.Get(ctx, u.ID, n.ID); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("absent banner: want ErrNodeBannerNotFound, got %v", err)
	}

	first := domain.NodeBanner{
		NodeID: n.ID, OwnerID: u.ID, Mime: "image/png",
		Ref: "aaaaaaaaaaaa", Bytes: []byte{1, 2, 3}, UpdatedAt: now,
	}
	if err := st.Put(ctx, first); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := st.Get(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Mime != "image/png" || got.Ref != "aaaaaaaaaaaa" || !bytes.Equal(got.Bytes, []byte{1, 2, 3}) {
		t.Errorf("get returned %+v, want the stored banner", got)
	}

	// Replace-on-upload: the second Put overwrites, it does not add a row.
	second := first
	second.Mime = "image/webp"
	second.Ref = "bbbbbbbbbbbb"
	second.Bytes = []byte{9, 9}
	second.UpdatedAt = now.Add(time.Minute)
	if err := st.Put(ctx, second); err != nil {
		t.Fatalf("replace put: %v", err)
	}
	replaced, err := st.Get(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatalf("get after replace: %v", err)
	}
	if replaced.Ref != "bbbbbbbbbbbb" || !bytes.Equal(replaced.Bytes, []byte{9, 9}) {
		t.Errorf("replace left %+v, want the second banner", replaced)
	}

	// Owner scope: a foreign owner sees nothing and deletes nothing.
	if _, err := st.Get(ctx, "u-fremd", n.ID); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("foreign get: want ErrNodeBannerNotFound, got %v", err)
	}
	if err := st.Delete(ctx, "u-fremd", n.ID); err != nil {
		t.Errorf("foreign delete must be a silent no-op, got %v", err)
	}
	if _, err := st.Get(ctx, u.ID, n.ID); err != nil {
		t.Fatalf("banner must survive a foreign delete, got %v", err)
	}

	if err := st.Delete(ctx, u.ID, n.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Get(ctx, u.ID, n.ID); !errors.Is(err, ports.ErrNodeBannerNotFound) {
		t.Errorf("after delete: want ErrNodeBannerNotFound, got %v", err)
	}
	if err := st.Delete(ctx, u.ID, n.ID); err != nil {
		t.Errorf("deleting an absent banner must be a no-op, got %v", err)
	}
}

// TestNodeBannerStore_PutIsOwnerScoped pins the upsert against a cross-tenant
// write. node_id is the primary key, so a second owner's Put collides with the
// first owner's row — without an owner clause on the conflict update it would
// silently overwrite another tenant's image while leaving owner_id untouched.
// AGENTS.md: every store query carries the ownerID, a cross-tenant leak is a
// Critical finding.
func TestNodeBannerStore_PutIsOwnerScoped(t *testing.T) {
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
	owner, _ := domain.NewUser("u-own", "sub-own", "own", "own@x.de", "Own")
	if _, err := users.UpsertBySub(ctx, owner); err != nil {
		t.Fatal(err)
	}
	intruder, _ := domain.NewUser("u-other", "sub-other", "other", "other@x.de", "Other")
	if _, err := users.UpsertBySub(ctx, intruder); err != nil {
		t.Fatal(err)
	}
	n, _ := domain.NewNode("eng-own", owner.ID, "Engagement Own", "eng-own", time.Now().UTC())
	n.Kind = domain.KindEngagement
	if _, err := pgstore.NewNodeStore(pool).Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeBannerStore(pool)
	now := time.Now().UTC().Truncate(time.Millisecond)

	mine := domain.NodeBanner{
		NodeID: n.ID, OwnerID: owner.ID, Mime: "image/png",
		Ref: "0000aaaa0000", Bytes: []byte{1, 1, 1}, UpdatedAt: now,
	}
	if err := st.Put(ctx, mine); err != nil {
		t.Fatalf("put: %v", err)
	}

	theirs := domain.NodeBanner{
		NodeID: n.ID, OwnerID: intruder.ID, Mime: "image/png",
		Ref: "ffffbbbbffff", Bytes: []byte{7, 7, 7}, UpdatedAt: now.Add(time.Minute),
	}
	_ = st.Put(ctx, theirs) // must not reach the row; an error here is fine too

	after, err := st.Get(ctx, owner.ID, n.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Ref != "0000aaaa0000" || !bytes.Equal(after.Bytes, []byte{1, 1, 1}) {
		t.Errorf("a foreign Put overwrote the owner's banner: %+v", after)
	}
}
