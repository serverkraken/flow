package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// ---- fakes ----

// fakeActiveSessionStore implements ports.ActiveSessionStore in memory.
type fakeActiveSessionStore struct {
	rows      map[string]domain.ActiveSession // key: userID+"/"+projectID
	upserted  []domain.ActiveSession
	deleted   [][2]string // [userID, projectID]
	getErr    error
	upsertErr error
	deleteErr error
}

func newFakeActiveSessionStore() *fakeActiveSessionStore {
	return &fakeActiveSessionStore{rows: map[string]domain.ActiveSession{}}
}

func (f *fakeActiveSessionStore) key(userID, projectID string) string {
	return userID + "/" + projectID
}

func (f *fakeActiveSessionStore) ListByUser(userID string) ([]domain.ActiveSession, error) {
	var out []domain.ActiveSession
	for _, row := range f.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeActiveSessionStore) Get(userID, projectID string) (domain.ActiveSession, error) {
	if f.getErr != nil {
		return domain.ActiveSession{}, f.getErr
	}
	row, ok := f.rows[f.key(userID, projectID)]
	if !ok {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	return row, nil
}

func (f *fakeActiveSessionStore) Upsert(a domain.ActiveSession) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[f.key(a.UserID, a.ProjectID)] = a
	f.upserted = append(f.upserted, a)
	return nil
}

func (f *fakeActiveSessionStore) Delete(userID, projectID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, f.key(userID, projectID))
	f.deleted = append(f.deleted, [2]string{userID, projectID})
	return nil
}

// fakeASProjectStore implements ports.ProjectStore for ActiveSessions tests.
type fakeASProjectStore struct {
	projects []domain.Project
	touched  []string
	touchErr error
}

func (f *fakeASProjectStore) ListActive(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeASProjectStore) ListAll(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeASProjectStore) GetByID(_, id string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeASProjectStore) GetBySlug(_, slug string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeASProjectStore) EnsureBySlug(_, name, slug string) (domain.Project, error) {
	p := domain.Project{ID: "auto-id", Name: name, Slug: slug, CreatedAt: time.Now()}
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeASProjectStore) Upsert(p domain.Project) error {
	f.projects = append(f.projects, p)
	return nil
}

func (f *fakeASProjectStore) TouchLastUsed(_, id string) error {
	if f.touchErr != nil {
		return f.touchErr
	}
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeASProjectStore) Archive(_, _ string) error {
	return nil
}

// fakeWorktimeMachine implements ports.WorktimeMachine in memory.
type fakeWorktimeMachine struct {
	startResult   domain.ActiveSession
	startErr      error
	stopResult    domain.Session
	stopErr       error
	pauseResult   domain.ActiveSession
	pauseErr      error
	resumeResult  domain.ActiveSession
	resumeErr     error
	correctResult domain.ActiveSession
	correctErr    error

	startCalls   []startCall
	stopCalls    []string // projectID
	pauseCalls   []string
	resumeCalls  []string
	correctCalls []correctCall
}

type startCall struct {
	projectID string
	tag       string
	note      string
}

type correctCall struct {
	projectID string
	ts        time.Time
}

func (f *fakeWorktimeMachine) Start(projectID, tag, note string) (domain.ActiveSession, error) {
	f.startCalls = append(f.startCalls, startCall{projectID, tag, note})
	return f.startResult, f.startErr
}

func (f *fakeWorktimeMachine) Stop(projectID string) (domain.Session, error) {
	f.stopCalls = append(f.stopCalls, projectID)
	return f.stopResult, f.stopErr
}

func (f *fakeWorktimeMachine) Pause(projectID string) (domain.ActiveSession, error) {
	f.pauseCalls = append(f.pauseCalls, projectID)
	return f.pauseResult, f.pauseErr
}

func (f *fakeWorktimeMachine) Resume(projectID string) (domain.ActiveSession, error) {
	f.resumeCalls = append(f.resumeCalls, projectID)
	return f.resumeResult, f.resumeErr
}

func (f *fakeWorktimeMachine) CorrectStart(projectID string, ts time.Time) (domain.ActiveSession, error) {
	f.correctCalls = append(f.correctCalls, correctCall{projectID, ts})
	return f.correctResult, f.correctErr
}

// ---- helpers ----

func mkActiveSessions(
	active *fakeActiveSessionStore,
	projects *fakeASProjectStore,
	machine *fakeWorktimeMachine,
) *usecase.ActiveSessions {
	return usecase.NewActiveSessions(nil, projects, active, machine)
}

// ---- Start tests ----

// Start happy-path: machine.Start is called with correct args; result is returned.
func TestUnit_ActiveSessions_Start_HappyPath(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	projects := &fakeASProjectStore{}
	machine := &fakeWorktimeMachine{
		startResult: domain.ActiveSession{
			UserID:    "u1",
			ProjectID: "p1",
			StartedAt: time.Now().UTC(),
			Tag:       "deep",
			Note:      "n1",
		},
	}

	uc := mkActiveSessions(active, projects, machine)

	row, err := uc.Start("u1", "p1", "deep", "n1")
	if err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if row.ProjectID != "p1" {
		t.Errorf("ProjectID: got %q, want p1", row.ProjectID)
	}
	if row.Tag != "deep" {
		t.Errorf("Tag: got %q, want deep", row.Tag)
	}
	if len(machine.startCalls) != 1 {
		t.Fatalf("expected 1 machine.Start call, got %d", len(machine.startCalls))
	}
	c := machine.startCalls[0]
	if c.projectID != "p1" || c.tag != "deep" || c.note != "n1" {
		t.Errorf("machine.Start args: %+v", c)
	}
}

// Start maps ErrActiveSessionConflict from machine → ErrActiveSessionExists.
func TestUnit_ActiveSessions_Start_ConflictMappedToExists(t *testing.T) {
	t.Parallel()
	machine := &fakeWorktimeMachine{startErr: ports.ErrActiveSessionConflict}
	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)

	_, err := uc.Start("u1", "p1", "", "")
	if !errors.Is(err, usecase.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists, got %v", err)
	}
}

// Start propagates unexpected errors from machine as-is.
func TestUnit_ActiveSessions_Start_OtherErrorBubbles(t *testing.T) {
	t.Parallel()
	unexpectedErr := errors.New("server error")
	machine := &fakeWorktimeMachine{startErr: unexpectedErr}
	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)

	_, err := uc.Start("u1", "p1", "", "")
	if !errors.Is(err, unexpectedErr) {
		t.Fatalf("expected server error to bubble, got %v", err)
	}
}

// ---- Stop tests ----

// Stop delegates to machine.Stop and returns the session.
func TestUnit_ActiveSessions_Stop_HappyPath(t *testing.T) {
	t.Parallel()
	startedAt := time.Now().UTC().Add(-30 * time.Minute)
	machine := &fakeWorktimeMachine{
		stopResult: domain.Session{
			ID:        "sess-1",
			UserID:    "u1",
			ProjectID: "p1",
			Start:     startedAt,
			Tag:       "deep",
		},
	}

	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)
	sess, err := uc.Stop("u1", "p1", "", "")
	if err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
	if sess.ID != "sess-1" {
		t.Errorf("session ID: got %q, want sess-1", sess.ID)
	}
	if len(machine.stopCalls) != 1 || machine.stopCalls[0] != "p1" {
		t.Errorf("machine.Stop calls: %v", machine.stopCalls)
	}
}

// Stop propagates errors from machine.
func TestUnit_ActiveSessions_Stop_Error(t *testing.T) {
	t.Parallel()
	machine := &fakeWorktimeMachine{stopErr: ports.ErrActiveSessionNotFound}
	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)

	_, err := uc.Stop("u1", "p1", "", "")
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Fatalf("expected ErrActiveSessionNotFound, got %v", err)
	}
}

// ---- Pause tests ----

// Pause delegates to machine.Pause.
func TestUnit_ActiveSessions_Pause_Delegates(t *testing.T) {
	t.Parallel()
	machine := &fakeWorktimeMachine{
		pauseResult: domain.ActiveSession{ProjectID: "p1"},
	}
	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)

	as, err := uc.Pause("u1", "p1")
	if err != nil {
		t.Fatalf("Pause: unexpected error: %v", err)
	}
	if as.ProjectID != "p1" {
		t.Errorf("ProjectID: got %q, want p1", as.ProjectID)
	}
	if len(machine.pauseCalls) != 1 || machine.pauseCalls[0] != "p1" {
		t.Errorf("machine.Pause calls: %v", machine.pauseCalls)
	}
}

// ---- Resume tests ----

// Resume delegates to machine.Resume.
func TestUnit_ActiveSessions_Resume_Delegates(t *testing.T) {
	t.Parallel()
	machine := &fakeWorktimeMachine{
		resumeResult: domain.ActiveSession{ProjectID: "p1"},
	}
	uc := mkActiveSessions(newFakeActiveSessionStore(), &fakeASProjectStore{}, machine)

	as, err := uc.Resume("u1", "p1")
	if err != nil {
		t.Fatalf("Resume: unexpected error: %v", err)
	}
	if as.ProjectID != "p1" {
		t.Errorf("ProjectID: got %q, want p1", as.ProjectID)
	}
	if len(machine.resumeCalls) != 1 || machine.resumeCalls[0] != "p1" {
		t.Errorf("machine.Resume calls: %v", machine.resumeCalls)
	}
}

// ---- ListActive tests ----

// ListActive delegates to the store and returns whatever it returns.
func TestUnit_ActiveSessions_ListActive_Delegates(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	_ = active.Upsert(domain.ActiveSession{UserID: "u1", ProjectID: "p1", StartedAt: time.Now().UTC()})
	_ = active.Upsert(domain.ActiveSession{UserID: "u1", ProjectID: "p2", StartedAt: time.Now().UTC()})
	active.upserted = nil

	uc := mkActiveSessions(active, &fakeASProjectStore{}, &fakeWorktimeMachine{})

	rows, err := uc.ListActive("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(rows))
	}
}

// ---- CorrectStart tests ----

// CorrectStart finds earliest session and calls machine.CorrectStart.
func TestCorrectStartMovesStartedAtAndCallsMachine(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	// Two sessions; p2 started earlier → should be selected.
	_ = active.Upsert(domain.ActiveSession{
		UserID: "u1", ProjectID: "p1",
		StartedAt: time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC),
	})
	_ = active.Upsert(domain.ActiveSession{
		UserID: "u1", ProjectID: "p2",
		StartedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
	})

	machine := &fakeWorktimeMachine{}
	uc := mkActiveSessions(active, &fakeASProjectStore{}, machine)

	ts := time.Date(2026, 6, 10, 8, 30, 0, 0, time.UTC)
	if err := uc.CorrectStart("u1", ts); err != nil {
		t.Fatalf("CorrectStart: %v", err)
	}

	if len(machine.correctCalls) != 1 {
		t.Fatalf("expected 1 machine.CorrectStart call, got %d", len(machine.correctCalls))
	}
	cc := machine.correctCalls[0]
	if cc.projectID != "p2" {
		t.Errorf("selected project: got %q, want p2 (earliest)", cc.projectID)
	}
	if !cc.ts.Equal(ts) {
		t.Errorf("ts: got %v, want %v", cc.ts, ts)
	}
}

// CorrectStart with no running sessions returns ErrActiveSessionNotFound.
func TestCorrectStartNothingRunning(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore() // empty
	machine := &fakeWorktimeMachine{}
	uc := mkActiveSessions(active, &fakeASProjectStore{}, machine)

	ts := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	err := uc.CorrectStart("u1", ts)
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Fatalf("expected ErrActiveSessionNotFound, got %v", err)
	}
	if len(machine.correctCalls) != 0 {
		t.Errorf("expected no machine.CorrectStart calls, got %d", len(machine.correctCalls))
	}
}
