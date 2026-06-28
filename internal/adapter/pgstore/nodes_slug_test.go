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

// slugTestStore spins up Postgres, migrates, seeds a user, and returns the store.
func slugTestStore(t *testing.T) (context.Context, *pgstore.NodeStore) {
	t.Helper()
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
	u, _ := domain.NewUser("u-slug", "sub-slug", "sluguser", "slug@x.de", "Slug User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	return ctx, pgstore.NewNodeStore(pool)
}

func mkNode(t *testing.T, ctx context.Context, s *pgstore.NodeStore, id, name, slug string, kind domain.NodeKind, parent *string) (domain.Node, error) {
	t.Helper()
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	n, err := domain.NewNode(id, "u-slug", name, slug, now)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	n.Kind = kind
	n.ParentID = parent
	return s.Create(ctx, n)
}

// TestNodeStore_SameSlugDifferentParentAllowed is the core capability: a repo may
// share its slug with its parent vorhaben (or any node in another subtree), because
// slugs are unique per sibling set, not globally.
func TestNodeStore_SameSlugDifferentParentAllowed(t *testing.T) {
	ctx, s := slugTestStore(t)
	if _, err := mkNode(t, ctx, s, "eng1", "Privat", "privat", domain.KindEngagement, nil); err != nil {
		t.Fatalf("create engagement: %v", err)
	}
	eng := "eng1"
	if _, err := mkNode(t, ctx, s, "vor1", "Straßenfuchs", "strassenfuchs", domain.KindVorhaben, &eng); err != nil {
		t.Fatalf("create vorhaben: %v", err)
	}
	vor := "vor1"
	if _, err := mkNode(t, ctx, s, "repo1", "Straßenfuchs", "strassenfuchs", domain.KindRepo, &vor); err != nil {
		t.Fatalf("repo with same slug as its parent vorhaben should be allowed, got: %v", err)
	}
}

// TestNodeStore_DuplicateSiblingSlug rejects two children of the same parent with
// the same slug — and surfaces the friendly ErrNodeSlugTaken sentinel, not a raw
// driver error.
func TestNodeStore_DuplicateSiblingSlug(t *testing.T) {
	ctx, s := slugTestStore(t)
	if _, err := mkNode(t, ctx, s, "eng1", "Privat", "privat", domain.KindEngagement, nil); err != nil {
		t.Fatalf("create engagement: %v", err)
	}
	eng := "eng1"
	if _, err := mkNode(t, ctx, s, "vor1", "API", "api", domain.KindVorhaben, &eng); err != nil {
		t.Fatalf("create first vorhaben: %v", err)
	}
	_, err := mkNode(t, ctx, s, "vor2", "API", "api", domain.KindVorhaben, &eng)
	if !errors.Is(err, ports.ErrNodeSlugTaken) {
		t.Fatalf("duplicate sibling slug: want ErrNodeSlugTaken, got %v", err)
	}
}

// TestNodeStore_DuplicateRootSlug rejects two engagements (roots, parent NULL) with
// the same slug — the partial unique index must treat NULL parents as one set.
func TestNodeStore_DuplicateRootSlug(t *testing.T) {
	ctx, s := slugTestStore(t)
	if _, err := mkNode(t, ctx, s, "eng1", "Privat", "privat", domain.KindEngagement, nil); err != nil {
		t.Fatalf("create first engagement: %v", err)
	}
	_, err := mkNode(t, ctx, s, "eng2", "Privat", "privat", domain.KindEngagement, nil)
	if !errors.Is(err, ports.ErrNodeSlugTaken) {
		t.Fatalf("duplicate root slug: want ErrNodeSlugTaken, got %v", err)
	}
}
