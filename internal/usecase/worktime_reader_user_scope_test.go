package usecase

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// userScopedSessions fails the test when Load is called with the wrong userID.
type userScopedSessions struct {
	wantUser string
	rows     []domain.Session
	t        *testing.T
}

func (s *userScopedSessions) Load(userID string) ([]domain.Session, error) {
	if userID != s.wantUser {
		s.t.Errorf("Load called with userID %q, want %q", userID, s.wantUser)
	}
	return s.rows, nil
}

func (s *userScopedSessions) LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, _ := s.Load(userID)
	var out []domain.Session
	for _, r := range all {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *userScopedSessions) Upsert(domain.Session) error        { return nil }
func (s *userScopedSessions) UpsertBatch([]domain.Session) error { return nil }
func (s *userScopedSessions) Delete(userID, id string) error     { return nil }

type fakeActiveList struct {
	t        *testing.T
	wantUser string
	rows     []domain.ActiveSession
}

func (f *fakeActiveList) ListByUser(userID string) ([]domain.ActiveSession, error) {
	if f.t != nil && userID != f.wantUser {
		f.t.Errorf("ListByUser called with %q, want %q", userID, f.wantUser)
	}
	return f.rows, nil
}

func (f *fakeActiveList) Get(string, string) (domain.ActiveSession, error) {
	return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
}
func (f *fakeActiveList) Upsert(domain.ActiveSession) error { return nil }
func (f *fakeActiveList) Delete(string, string) error       { return nil }

type fakeIdleState struct{}

func (fakeIdleState) GetActive() (*time.Time, error) { return nil, nil }
func (fakeIdleState) SetActive(time.Time) error      { return nil }
func (fakeIdleState) ClearActive() error             { return nil }
func (fakeIdleState) GetPause() (*time.Time, error)  { return nil, nil }
func (fakeIdleState) SetPause(time.Time) error       { return nil }
func (fakeIdleState) ClearPause() error              { return nil }

func TestReaderTodayIsUserScopedAndReadsActiveStore(t *testing.T) {
	now := time.Date(2026, 6, 10, 14, 0, 0, 0, time.Local)
	started := now.Add(-25 * time.Minute).UTC()
	sessions := &userScopedSessions{
		wantUser: "user-1",
		t:        t,
		rows: []domain.Session{{
			ID: "s1", UserID: "user-1", ProjectID: "p1",
			Date:    time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local),
			Elapsed: time.Hour,
		}},
	}
	r := &WorktimeReader{
		Sessions: sessions,
		State:    fakeIdleState{},
		Active: &fakeActiveList{
			t:        t,
			wantUser: "user-1",
			rows:     []domain.ActiveSession{{UserID: "user-1", ProjectID: "p1", StartedAt: started}},
		},
		UserID: "user-1",
		Targets: &TargetResolver{
			Config:        &testutil.FakeConfigReader{Cfg: domain.Config{DefaultTarget: 8 * time.Hour}},
			DayOffs:       testutil.NewFakeDayOffStore(),
			DefaultTarget: 8 * time.Hour,
		},
		Clock: &testutil.FixedClock{T: now},
	}
	day, err := r.Today()
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if len(day.Sessions) != 1 {
		t.Errorf("want 1 session, got %d (userID scoping broken)", len(day.Sessions))
	}
	if day.Active == nil {
		t.Fatal("day.Active is nil — Active store not consulted")
	}
	if !day.Active.Equal(started) {
		t.Errorf("day.Active = %v, want %v", *day.Active, started)
	}
}

func TestReaderTodayActiveStoreEmptyHasNoRunning(t *testing.T) {
	now := time.Date(2026, 6, 10, 14, 0, 0, 0, time.Local)
	sessions := &userScopedSessions{
		wantUser: "user-1",
		t:        t,
		rows:     nil,
	}
	r := &WorktimeReader{
		Sessions: sessions,
		State:    fakeIdleState{},
		Active:   &fakeActiveList{t: t, wantUser: "user-1", rows: nil},
		UserID:   "user-1",
		Targets: &TargetResolver{
			Config:        &testutil.FakeConfigReader{Cfg: domain.Config{DefaultTarget: 8 * time.Hour}},
			DayOffs:       testutil.NewFakeDayOffStore(),
			DefaultTarget: 8 * time.Hour,
		},
		Clock: &testutil.FixedClock{T: now},
	}
	day, err := r.Today()
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if day.Active != nil {
		t.Errorf("day.Active should be nil with empty active store, got %v", *day.Active)
	}
}
