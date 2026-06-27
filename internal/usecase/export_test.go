package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeNodeStore is a minimal in-memory NodeStore for export tests.
type fakeNodeStore struct {
	list    []domain.Node
	setRate *domain.Money // last rate set via SetRate (for assertion, not used here)
}

func (f fakeNodeStore) Create(_ context.Context, p domain.Node) (domain.Node, error) {
	return p, nil
}
func (f fakeNodeStore) List(_ context.Context, _ string) ([]domain.Node, error) {
	return f.list, nil
}
func (f fakeNodeStore) Get(_ context.Context, _, id string) (domain.Node, error) {
	for _, p := range f.list {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Node{}, nil
}
func (f *fakeNodeStore) Update(_ context.Context, _ string, p domain.Node) (domain.Node, error) {
	return p, nil
}
func (f *fakeNodeStore) SetRate(_ context.Context, _, _ string, rate *domain.Money) error {
	f.setRate = rate
	return nil
}
func (f *fakeNodeStore) Delete(_ context.Context, _, _ string) error { return nil }

func TestBuildExport_AggregatesByProject(t *testing.T) {
	loc := time.UTC
	pid := "p1"
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	sessions := []domain.WorkSession{
		{
			ID:        "a",
			NodeID: &pid,
			Start:     time.Date(2026, 6, 15, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, loc)), // 2h
		},
		{
			ID:        "b",
			NodeID: &pid,
			Start:     time.Date(2026, 6, 15, 12, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 12, 30, 0, 0, loc)), // 30m
		},
		{
			ID:        "run",
			NodeID: &pid,
			Start:     time.Date(2026, 6, 15, 13, 0, 0, 0, loc),
			Stop:      nil, // running — must be excluded
		},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Nodes: &fakeNodeStore{list: []domain.Node{{ID: pid, Name: "Acme", Rate: &rate}}},
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
	if data.ByEngagement[0].Amount == nil || data.ByEngagement[0].Amount.Amount != 20000 {
		t.Errorf("amount: got %+v want 20000 (2.5h*8000)", data.ByEngagement[0].Amount)
	}
}

func TestBuildExport_ExcludesOutOfRange(t *testing.T) {
	loc := time.UTC
	pid := "p1"
	sessions := []domain.WorkSession{
		{
			ID:        "before",
			NodeID: &pid,
			Start:     time.Date(2026, 5, 31, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 5, 31, 10, 0, 0, 0, loc)),
		},
		{
			ID:        "in",
			NodeID: &pid,
			Start:     time.Date(2026, 6, 15, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc)),
		},
		{
			ID:        "after",
			NodeID: &pid,
			Start:     time.Date(2026, 7, 1, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 7, 1, 10, 0, 0, 0, loc)),
		},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Nodes: &fakeNodeStore{list: []domain.Node{{ID: pid, Name: "X"}}},
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
		t.Fatalf("want 1 in-range session, got %d", len(data.Sessions))
	}
}

func TestSetProjectRate_Validates(t *testing.T) {
	uc := usecase.SetNodeRate{Nodes: &fakeNodeStore{}}
	if err := uc.Execute(context.Background(), "u1", "p1", &domain.Money{Amount: -1, Currency: "EUR"}); err == nil {
		t.Error("negative amount should error")
	}
	if err := uc.Execute(context.Background(), "u1", "p1", &domain.Money{Amount: 1, Currency: "EU"}); err == nil {
		t.Error("bad currency (2 chars) should error")
	}
	// nil rate (clear) should succeed
	if err := uc.Execute(context.Background(), "u1", "p1", nil); err != nil {
		t.Errorf("nil rate (clear) should not error: %v", err)
	}
	// valid rate should succeed
	if err := uc.Execute(context.Background(), "u1", "p1", &domain.Money{Amount: 5000, Currency: "EUR"}); err != nil {
		t.Errorf("valid rate should not error: %v", err)
	}
}

// TestBuildExport_WithExplicitLoc covers the loc() != nil branch (BuildExport.Loc set).
// The existing tests already pass Loc (non-nil), but this one makes the nil path
// explicit via Loc=nil to ensure both branches are exercised across test runs.
func TestBuildExport_NilLoc(t *testing.T) {
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{}, // no sessions → empty export
		Nodes: &fakeNodeStore{},
		Clock:    fixedClock{t: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)},
		Loc:      nil, // nil → uses time.Local; covers the else branch
	}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	_, err := uc.Execute(context.Background(), "u1", from, to, "")
	if err != nil {
		t.Fatalf("Execute with nil loc: %v", err)
	}
}
