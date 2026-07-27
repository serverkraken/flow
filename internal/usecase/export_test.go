package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBuildExport_AggregatesByEngagement(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	ns := testutil.NewFakeNodeStore()
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: "eng1", OwnerID: "u1", Kind: domain.KindEngagement,
		Name: "RTL Extern", Slug: "rtl-extern", Status: domain.NodeActive, Rate: &rate,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	eng := "eng1"
	sessions := []domain.WorkSession{
		{ID: "a", NodeID: &eng, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, loc))},   // 2h
		{ID: "b", NodeID: &eng, Start: time.Date(2026, 6, 15, 12, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 12, 30, 0, 0, loc))}, // 30m
		{ID: "run", NodeID: &eng, Start: time.Date(2026, 6, 15, 13, 0, 0, 0, loc), Stop: nil},                                          // running → excluded
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Nodes:    ns,
		Clock:    fixedClock{t: time.Date(2026, 6, 16, 0, 0, 0, 0, loc)},
		Loc:      loc,
	}
	data, err := uc.Execute(context.Background(), "u1",
		time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 6, 30, 0, 0, 0, 0, loc),
		"")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sessions) != 2 {
		t.Fatalf("want 2 detail rows (running excluded), got %d", len(data.Sessions))
	}
	if len(data.ByEngagement) != 1 || data.ByEngagement[0].Total != 150*time.Minute {
		t.Fatalf("aggregate: got %+v", data.ByEngagement)
	}
	if data.ByEngagement[0].NodeName != "RTL Extern" {
		t.Errorf("name: got %q", data.ByEngagement[0].NodeName)
	}
	if data.ByEngagement[0].Amount == nil || data.ByEngagement[0].Amount.Amount != 20000 {
		t.Errorf("amount: got %+v want 20000 (2.5h*8000)", data.ByEngagement[0].Amount)
	}
}

func TestBuildExport_ExcludesOutOfRange(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	ns := testutil.NewFakeNodeStore()
	_, _ = ns.Create(context.Background(), domain.Node{
		ID: "eng1", OwnerID: "u1", Kind: domain.KindEngagement,
		Name: "X", Slug: "x", Status: domain.NodeActive,
	})
	eng := "eng1"
	sessions := []domain.WorkSession{
		{ID: "before", NodeID: &eng, Start: time.Date(2026, 5, 31, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 5, 31, 10, 0, 0, 0, loc))},
		{ID: "in", NodeID: &eng, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc))},
		{ID: "after", NodeID: &eng, Start: time.Date(2026, 7, 1, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 7, 1, 10, 0, 0, 0, loc))},
		{ID: "unbooked", NodeID: nil, Start: time.Date(2026, 6, 16, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 16, 10, 0, 0, 0, loc))},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Nodes:    ns,
		Clock:    fixedClock{t: time.Date(2026, 6, 20, 0, 0, 0, 0, loc)},
		Loc:      loc,
	}
	data, err := uc.Execute(context.Background(), "u1",
		time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 6, 30, 0, 0, 0, 0, loc),
		"")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("want 1 in-range booked session, got %d", len(data.Sessions))
	}
}

func TestBuildExport_FilterByEngagement(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	ns := testutil.NewFakeNodeStore()
	_, _ = ns.Create(context.Background(), domain.Node{ID: "e1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "A", Slug: "a", Status: domain.NodeActive})
	_, _ = ns.Create(context.Background(), domain.Node{ID: "e2", OwnerID: "u1", Kind: domain.KindEngagement, Name: "B", Slug: "b", Status: domain.NodeActive})
	e1, e2 := "e1", "e2"
	sessions := []domain.WorkSession{
		{ID: "a", NodeID: &e1, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc))},
		{ID: "b", NodeID: &e2, Start: time.Date(2026, 6, 15, 11, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 12, 0, 0, 0, loc))},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Nodes:    ns,
		Clock:    fixedClock{t: time.Date(2026, 6, 16, 0, 0, 0, 0, loc)},
		Loc:      loc,
	}
	data, err := uc.Execute(context.Background(), "u1",
		time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 6, 30, 0, 0, 0, 0, loc),
		"e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.ByEngagement) != 1 || data.ByEngagement[0].NodeID != "e1" {
		t.Fatalf("filter: got %+v", data.ByEngagement)
	}
}

// TestBuildExport_NilLoc covers the loc() nil-fallback branch (BuildExport.Loc unset).
func TestBuildExport_NilLoc(t *testing.T) {
	t.Parallel()
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{}, // no sessions → empty export
		Nodes:    testutil.NewFakeNodeStore(),
		Clock:    fixedClock{t: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)},
		Loc:      nil, // nil → uses time.Local; covers the else branch
	}
	if _, err := uc.Execute(context.Background(), "u1",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		""); err != nil {
		t.Fatalf("Execute with nil loc: %v", err)
	}
}

func TestBuildExport_RateInheritedFromAncestor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	rate := domain.Money{Amount: 9500, Currency: "EUR"}
	if _, err := ns.Create(ctx, domain.Node{
		ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement,
		Name: "Kunde A", Slug: "kunde-a", Status: domain.NodeActive, Rate: &rate,
	}); err != nil {
		t.Fatalf("seed eng: %v", err)
	}
	parent := "eng"
	if _, err := ns.Create(ctx, domain.Node{
		ID: "repo", OwnerID: "u1", ParentID: &parent, Kind: domain.KindRepo,
		Name: "repo-y", Slug: "repo-y", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC) // 2h on the repo
	repo := "repo"
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", NodeID: &repo, Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	uc := usecase.BuildExport{Sessions: ss, Nodes: ns, Clock: testutil.FakeClock{T: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)}, Loc: time.UTC}
	data, err := uc.Execute(ctx, "u1", start, stop, "")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data.ByEngagement) != 1 {
		t.Fatalf("want 1 node total, got %d", len(data.ByEngagement))
	}
	nt := data.ByEngagement[0]
	if nt.Rate == nil || nt.Rate.Amount != 9500 {
		t.Fatalf("want inherited rate 9500, got %+v", nt.Rate)
	}
	if nt.Amount == nil || nt.Amount.Amount != 19000 { // 9500/h * 2h
		t.Fatalf("want amount 19000, got %+v", nt.Amount)
	}
}
