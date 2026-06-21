package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeProjectStore is a minimal in-memory ProjectStore for export tests.
type fakeProjectStore struct {
	list    []domain.Project
	setRate *domain.Money // last rate set via SetRate (for assertion, not used here)
}

func (f fakeProjectStore) Create(_ context.Context, p domain.Project) (domain.Project, error) {
	return p, nil
}
func (f fakeProjectStore) List(_ context.Context, _ string) ([]domain.Project, error) {
	return f.list, nil
}
func (f fakeProjectStore) Get(_ context.Context, _, id string) (domain.Project, error) {
	for _, p := range f.list {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, nil
}
func (f *fakeProjectStore) SetRate(_ context.Context, _, _ string, rate *domain.Money) error {
	f.setRate = rate
	return nil
}
func (f *fakeProjectStore) Delete(_ context.Context, _, _ string) error { return nil }

func TestBuildExport_AggregatesByProject(t *testing.T) {
	loc := time.UTC
	pid := "p1"
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	sessions := []domain.WorkSession{
		{
			ID:        "a",
			ProjectID: &pid,
			Start:     time.Date(2026, 6, 15, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, loc)), // 2h
		},
		{
			ID:        "b",
			ProjectID: &pid,
			Start:     time.Date(2026, 6, 15, 12, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 12, 30, 0, 0, loc)), // 30m
		},
		{
			ID:        "run",
			ProjectID: &pid,
			Start:     time.Date(2026, 6, 15, 13, 0, 0, 0, loc),
			Stop:      nil, // running — must be excluded
		},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Projects: &fakeProjectStore{list: []domain.Project{{ID: pid, Name: "Acme", Rate: &rate}}},
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
	if len(data.ByProject) != 1 || data.ByProject[0].Total != 150*time.Minute {
		t.Fatalf("aggregate: got %+v", data.ByProject)
	}
	if data.ByProject[0].Amount == nil || data.ByProject[0].Amount.Amount != 20000 {
		t.Errorf("amount: got %+v want 20000 (2.5h*8000)", data.ByProject[0].Amount)
	}
}

func TestBuildExport_ExcludesOutOfRange(t *testing.T) {
	loc := time.UTC
	pid := "p1"
	sessions := []domain.WorkSession{
		{
			ID:        "before",
			ProjectID: &pid,
			Start:     time.Date(2026, 5, 31, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 5, 31, 10, 0, 0, 0, loc)),
		},
		{
			ID:        "in",
			ProjectID: &pid,
			Start:     time.Date(2026, 6, 15, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc)),
		},
		{
			ID:        "after",
			ProjectID: &pid,
			Start:     time.Date(2026, 7, 1, 9, 0, 0, 0, loc),
			Stop:      ptr(time.Date(2026, 7, 1, 10, 0, 0, 0, loc)),
		},
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Projects: &fakeProjectStore{list: []domain.Project{{ID: pid, Name: "X"}}},
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
	uc := usecase.SetProjectRate{Projects: &fakeProjectStore{}}
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
