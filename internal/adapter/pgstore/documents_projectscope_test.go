package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func strptr(s string) *string { return &s }

// vec768 returns a 768-dim unit vector with 1.0 at index i (matches the
// migration 0010 vector(768) column).
func vec768(i int) []float32 {
	v := make([]float32, 768)
	v[i] = 1
	return v
}

func seedProjectScope(t *testing.T) (st *pgstore.DocumentStore, owner, pA, pB string) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-scope", "sub-scope", "scopeuser", "scope@x.de", "Scope User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	owner = "u-scope"
	ps := pgstore.NewNodeStore(pool)
	now := time.Now()
	a, err := domain.NewNode("proj-a", owner, "Alpha", "alpha", now)
	if err != nil {
		t.Fatalf("new project a: %v", err)
	}
	a.Kind = domain.KindEngagement
	if _, err := ps.Create(ctx, a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := domain.NewNode("proj-b", owner, "Beta", "beta", now)
	if err != nil {
		t.Fatalf("new project b: %v", err)
	}
	b.Kind = domain.KindEngagement
	if _, err := ps.Create(ctx, b); err != nil {
		t.Fatalf("create b: %v", err)
	}
	st = pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	mk := func(id, path string, proj *string, ty domain.DocumentType, body string) {
		_, err := st.Create(ctx, domain.Document{
			ID: id, OwnerID: owner, NodeID: proj, Type: ty,
			Path: path, Title: id, Body: body, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", id, err)
		}
	}
	mk("d-a", "alpha/note", strptr("proj-a"), domain.DocProject, "alpha widget design")
	mk("d-b", "beta/note", strptr("proj-b"), domain.DocProject, "beta widget design")
	mk("d-x", "free/note", nil, domain.DocFree, "unassigned widget design")
	return st, owner, "proj-a", "proj-b"
}

func ids(docs []domain.Document) map[string]bool {
	m := map[string]bool{}
	for _, d := range docs {
		m[d.ID] = true
	}
	return m
}

func TestDocumentStore_ListProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)

	all, err := st.List(ctx, owner, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("List(nil) = %d docs, %v; want 3", len(all), err)
	}
	only, err := st.List(ctx, owner, &pA)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("List(proj-a) = %v, %v; want [d-a]", ids(only), err)
	}
	none, err := st.List(ctx, owner, strptr("none"))
	if err != nil || len(none) != 1 || none[0].ID != "d-x" {
		t.Fatalf("List(none) = %v, %v; want [d-x]", ids(none), err)
	}
}

func TestDocumentStore_SearchProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)

	all, err := st.Search(ctx, owner, "widget", nil, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("Search(nil) = %d hits, %v; want 3", len(all), err)
	}
	only, err := st.Search(ctx, owner, "widget", &pA, nil)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("Search(proj-a) = %d hits, %v; want [d-a]", len(only), err)
	}
}

func TestDocumentStore_SemanticSearchProjectScope(t *testing.T) {
	ctx := context.Background()
	st, owner, pA, _ := seedProjectScope(t)
	// give each doc one chunk so SemanticSearch has candidates
	if err := st.ReplaceChunks(ctx, "d-a", owner, snapshotHash(t, st, "d-a"), []string{"alpha"}, [][]float32{vec768(0)}); err != nil {
		t.Fatalf("chunks d-a: %v", err)
	}
	if err := st.ReplaceChunks(ctx, "d-b", owner, snapshotHash(t, st, "d-b"), []string{"beta"}, [][]float32{vec768(1)}); err != nil {
		t.Fatalf("chunks d-b: %v", err)
	}

	all, err := st.SemanticSearch(ctx, owner, vec768(0), nil, nil, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("SemanticSearch(nil) = %d hits, %v; want 2", len(all), err)
	}
	only, err := st.SemanticSearch(ctx, owner, vec768(0), &pA, nil, 10)
	if err != nil || len(only) != 1 || only[0].ID != "d-a" {
		t.Fatalf("SemanticSearch(proj-a) = %d hits, %v; want [d-a]", len(only), err)
	}
}
