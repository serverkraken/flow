package pgstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestSessionStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// A session FKs to users(id) and projects(id), so seed a user + project.
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	projects := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	if _, err := projects.Create(ctx, p); err != nil {
		t.Fatalf("project create: %v", err)
	}

	sessions := pgstore.NewSessionStore(pool)
	if _, ok, err := sessions.Running(ctx, "u1"); err != nil || ok {
		t.Fatalf("expected no running session, got ok=%v err=%v", ok, err)
	}
	s, _ := domain.NewWorkSession("s1", "u1", nil, now)
	if _, err := sessions.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}
	s2, _ := domain.NewWorkSession("s2", "u1", nil, now.Add(time.Minute))
	if _, err := sessions.Create(ctx, s2); err == nil {
		t.Fatal("expected unique-violation for a second running session")
	}
	got, ok, err := sessions.Running(ctx, "u1")
	if err != nil || !ok || got.ID != "s1" {
		t.Fatalf("Running = %+v ok=%v err=%v", got, ok, err)
	}
	pid := "p1"
	stopAt := now.Add(time.Hour)
	stopped, err := sessions.Stop(ctx, "u1", "s1", &pid, stopAt)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Stop == nil || !stopped.Stop.Equal(stopAt) || stopped.ProjectID == nil || *stopped.ProjectID != "p1" {
		t.Fatalf("stop result wrong: %+v", stopped)
	}
	if _, ok, _ := sessions.Running(ctx, "u1"); ok {
		t.Fatal("nothing should be running after stop")
	}
	list, err := sessions.List(ctx, "u1", now.Add(-24*time.Hour))
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d sessions err=%v", len(list), err)
	}
	if _, err := sessions.Stop(ctx, "u1", "nope", &pid, stopAt); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}

	// Update: overwrite tag/note (and project/times) — owner-scoped
	updated, err := sessions.Update(ctx, "u1", "s1", &pid, "focus", "revised", now, &stopAt)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Tag != "focus" || updated.Note != "revised" {
		t.Fatalf("Update did not persist: %+v", updated)
	}
	// foreign-owner Update -> not found
	if _, err := sessions.Update(ctx, "nobody", "s1", nil, "", "", now, &stopAt); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign Update: want ErrSessionNotFound, got %v", err)
	}
	// Delete -> ok, then double-delete -> not found
	if err := sessions.Delete(ctx, "u1", "s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := sessions.Delete(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("double Delete: want ErrSessionNotFound, got %v", err)
	}
}

func TestSessionStore_ListRange(t *testing.T) {
	pool, err := pgstore.NewPool(context.Background(), startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	// Seed a user so FK on work_sessions.owner_id is satisfied.
	users := pgstore.NewUserStore(pool)
	owner := "u-range-" + t.Name()
	u, _ := domain.NewUser(owner, "sub-range", "range-user", "range@x.de", "Range")
	if _, err := users.UpsertBySub(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	store := pgstore.NewSessionStore(pool)
	ctx := context.Background()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: owner, Start: start, Stop: &stop, CreatedAt: start}
	}
	for _, ws := range []domain.WorkSession{mk("r-a", 8), mk("r-b", 10), mk("r-c", 23)} {
		if _, err := store.Create(ctx, ws); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := store.ListRange(ctx, owner,
		time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(got) != 1 || got[0].ID != "r-b" {
		t.Fatalf("ListRange = %+v, want only r-b", got)
	}
}

func TestSessionStore_ListPage(t *testing.T) {
	pool, err := pgstore.NewPool(context.Background(), startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	// Seed a user so FK on work_sessions.owner_id is satisfied.
	users := pgstore.NewUserStore(pool)
	owner := "u-page-" + t.Name()
	u, _ := domain.NewUser(owner, "sub-page", "page-user", "page@x.de", "Page")
	if _, err := users.UpsertBySub(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	store := pgstore.NewSessionStore(pool)
	ctx := context.Background()
	base := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		st := base.Add(time.Duration(i) * time.Hour)
		sp := st.Add(30 * time.Minute)
		if _, err := store.Create(ctx, domain.WorkSession{
			ID: fmt.Sprintf("p%d", i), OwnerID: owner, Start: st, Stop: &sp, CreatedAt: st,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// Seed a foreign-owner session — must not affect owner's total or page.
	foreignOwner := "u-page-foreign-" + t.Name()
	uf, _ := domain.NewUser(foreignOwner, "sub-page-foreign", "foreign-user", "foreign@x.de", "Foreign")
	if _, err := users.UpsertBySub(ctx, uf); err != nil {
		t.Fatalf("seed foreign user: %v", err)
	}
	fst := base.Add(4 * time.Hour)
	fsp := fst.Add(30 * time.Minute)
	if _, err := store.Create(ctx, domain.WorkSession{
		ID: "pF", OwnerID: foreignOwner, Start: fst, Stop: &fsp, CreatedAt: fst,
	}); err != nil {
		t.Fatalf("seed foreign session: %v", err)
	}

	items, total, err := store.ListPage(ctx, owner, 2, 0)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 3 and 2", total, len(items))
	}
	if !items[0].Start.After(items[1].Start) {
		t.Fatalf("not newest-first: %+v", items)
	}
}

func TestProjectStoreListOwnerScoped(t *testing.T) {
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
	ua, _ := domain.NewUser("ua", "sa", "a", "a@x", "A")
	ub, _ := domain.NewUser("ub", "sb", "b", "b@x", "B")
	_, _ = users.UpsertBySub(ctx, ua)
	_, _ = users.UpsertBySub(ctx, ub)
	ps := pgstore.NewProjectStore(pool)
	now := time.Now()
	pa, _ := domain.NewProject("pa", "ua", "A proj", "a-proj", now)
	pb, _ := domain.NewProject("pb", "ub", "B proj", "b-proj", now)
	_, _ = ps.Create(ctx, pa)
	_, _ = ps.Create(ctx, pb)
	list, err := ps.List(ctx, "ua")
	if err != nil || len(list) != 1 || list[0].ID != "pa" {
		t.Fatalf("owner-scoped list failed: %+v err=%v", list, err)
	}
	if _, err := ps.Get(ctx, "ua", "pb"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("cross-owner Get must be ErrProjectNotFound, got %v", err)
	}
}
