package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeSessionStore_ListRange(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: "u1", Start: start, Stop: &stop}
	}
	for _, ws := range []domain.WorkSession{mk("a", 8), mk("b", 10), mk("c", 23)} {
		if _, err := ss.Create(ctx, ws); err != nil {
			t.Fatalf("seed %s: %v", ws.ID, err)
		}
	}
	since := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	until := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	got, err := ss.ListRange(ctx, "u1", since, until)
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("ListRange = %+v, want only b", got)
	}
	// foreign owner sees nothing
	if g, _ := ss.ListRange(ctx, "other", since, until); len(g) != 0 {
		t.Fatalf("foreign ListRange = %+v, want empty", g)
	}
}

func TestFakeSessionStore_ListPage(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: "u1", Start: start, Stop: &stop}
	}
	for _, ws := range []domain.WorkSession{mk("a", 8), mk("b", 10), mk("c", 12)} {
		if _, err := ss.Create(ctx, ws); err != nil {
			t.Fatalf("seed %s: %v", ws.ID, err)
		}
	}
	// foreign owner must not count
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "x", OwnerID: "u2",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed foreign: %v", err)
	}
	items, total, err := ss.ListPage(ctx, "u1", 2, 0)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(items) != 2 || items[0].ID != "c" || items[1].ID != "b" {
		t.Fatalf("page1 = %+v, want [c b] newest-first", items)
	}
	page2, _, _ := ss.ListPage(ctx, "u1", 2, 2)
	if len(page2) != 1 || page2[0].ID != "a" {
		t.Fatalf("page2 = %+v, want [a]", page2)
	}
}

func TestFakeSessionStore_UpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	seed := domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}
	if _, err := ss.Create(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pid := "p1"
	got, err := ss.Update(ctx, "u1", "s1", &pid, "deep", "note", start, &stop)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Tag != "deep" || got.Note != "note" || got.NodeID == nil || *got.NodeID != "p1" {
		t.Fatalf("update did not persist fields: %+v", got)
	}

	// non-existent id -> not found
	if _, err := ss.Update(ctx, "u1", "no-such-id", nil, "", "", start, &stop); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("missing id update: want ErrSessionNotFound, got %v", err)
	}
	// foreign owner -> not found
	if _, err := ss.Update(ctx, "other", "s1", nil, "", "", start, &stop); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign update: want ErrSessionNotFound, got %v", err)
	}
	// delete foreign -> not found
	if err := ss.Delete(ctx, "other", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign delete: want ErrSessionNotFound, got %v", err)
	}
	// delete owner -> ok, then gone
	if err := ss.Delete(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := ss.Delete(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("double delete: want ErrSessionNotFound, got %v", err)
	}
}
