package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNodeMRU_SortedNewestFirst(t *testing.T) {
	sessions := testutil.NewFakeSessionStore()
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mk := func(id, node string, start time.Time) {
		stop := start.Add(time.Hour)
		_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: "u1", NodeID: &node, Start: start, Stop: &stop})
	}
	mk("a", "n-old", base)
	mk("b", "n-new", base.AddDate(0, 0, 10))
	out, err := usecase.NodeMRU{Sessions: sessions}.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].NodeID != "n-new" || out[1].NodeID != "n-old" {
		t.Fatalf("want newest-first [n-new, n-old], got %+v", out)
	}
}

func TestNodeMRU_EmptyWhenNoBookings(t *testing.T) {
	sessions := testutil.NewFakeSessionStore()
	out, err := usecase.NodeMRU{Sessions: sessions}.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("no bookings → empty, got %+v", out)
	}
}
