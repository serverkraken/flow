package pgstore_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestSessionStore_DatabaseRejectsConcurrentOverlap(t *testing.T) {
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
	u, _ := domain.NewUser("u-overlap", "sub-overlap", "overlap", "overlap@example.test", "Overlap")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewSessionStore(pool)
	start := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)

	ready := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-ready
			_, err := store.Create(ctx, domain.WorkSession{ID: id, OwnerID: u.ID, Start: start, Stop: &stop, CreatedAt: start})
			results <- err
		}(id)
	}
	close(ready)
	wg.Wait()
	close(results)

	var succeeded, overlapped int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrOverlap):
			overlapped++
		default:
			t.Fatalf("unexpected write error: %v", err)
		}
	}
	if succeeded != 1 || overlapped != 1 {
		t.Fatalf("succeeded=%d overlapped=%d, want 1/1", succeeded, overlapped)
	}

	adjacentStart := stop
	adjacentStop := adjacentStart.Add(time.Hour)
	if _, err := store.Create(ctx, domain.WorkSession{
		ID: "adjacent", OwnerID: u.ID, Start: adjacentStart, Stop: &adjacentStop, CreatedAt: adjacentStart,
	}); err != nil {
		t.Fatalf("adjacent [start,stop) interval rejected: %v", err)
	}

	errInjected := errors.New("rollback aggregate")
	rollbackStart := adjacentStop
	err = store.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		if _, err := tx.Create(ctx, domain.WorkSession{
			ID: "rolled-back", OwnerID: u.ID, Start: rollbackStart, CreatedAt: rollbackStart,
		}); err != nil {
			return err
		}
		if _, err := tx.SetTags(ctx, u.ID, "rolled-back", []string{"Deep Work"}); err != nil {
			return err
		}
		return errInjected
	})
	if !errors.Is(err, errInjected) {
		t.Fatalf("transaction error=%v, want injected rollback", err)
	}
	if _, err := store.Get(ctx, u.ID, "rolled-back"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("session survived aggregate rollback: %v", err)
	}
	var tagCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tags WHERE owner_id=$1 AND slug='deep-work'`, u.ID).Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 0 {
		t.Fatalf("tag survived aggregate rollback: count=%d", tagCount)
	}
}

func TestOverlapMigration_RejectsExistingOverlapWithoutChangingRows(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.MigrateUpTo(ctx, pool, 32); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-overlap-migration", "sub-overlap-migration", "overlap-migration", "migration@example.test", "Migration")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	for _, id := range []string{"existing-one", "existing-two"} {
		if _, err := pool.Exec(ctx, `INSERT INTO work_sessions
(id, owner_id, start_at, stop_at, created_at) VALUES ($1,$2,$3,$4,$3)`, id, u.ID, start, stop); err != nil {
			t.Fatal(err)
		}
	}

	if err := pgstore.Migrate(ctx, pool); err == nil {
		t.Fatal("migration accepted pre-existing overlapping sessions")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM work_sessions WHERE owner_id=$1`, u.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migration changed existing sessions: count=%d, want 2", count)
	}
	var constraintExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
SELECT 1 FROM pg_constraint WHERE conname='work_sessions_no_overlap'
)`).Scan(&constraintExists); err != nil {
		t.Fatal(err)
	}
	if constraintExists {
		t.Fatal("failed migration left a partially installed overlap constraint")
	}
}
