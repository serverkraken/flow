package testutil

// FakeWorktimeMachine is an in-memory ports.WorktimeMachine for unit tests.
// It delegates Start/Stop to FakeActiveSessionStoreV2 and FakeSessionStore
// so tests can assert on the resulting rows without a real server.

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// FakeWorktimeMachine implements ports.WorktimeMachine using in-memory stores.
// Construct via NewFakeWorktimeMachine.
type FakeWorktimeMachine struct {
	Active   *FakeActiveSessionStoreV2
	Sessions *FakeSessionStore
	UserID   string

	// StartErr, when non-nil, is returned by Start instead of normal logic.
	StartErr error
	// StopErr, when non-nil, is returned by Stop instead of normal logic.
	StopErr error
}

// NewFakeWorktimeMachine constructs a FakeWorktimeMachine with the given stores.
// userID is used to scope active-session key lookups.
func NewFakeWorktimeMachine(userID string, active *FakeActiveSessionStoreV2, sessions *FakeSessionStore) *FakeWorktimeMachine {
	return &FakeWorktimeMachine{
		Active:   active,
		Sessions: sessions,
		UserID:   userID,
	}
}

// Start creates a new active session. Returns ErrActiveSessionConflict when one
// already exists for (userID, projectID) — maps to ErrActiveSessionExists upstream.
func (m *FakeWorktimeMachine) Start(projectID, tag, note string) (domain.ActiveSession, error) {
	if m.StartErr != nil {
		return domain.ActiveSession{}, m.StartErr
	}
	if _, err := m.Active.Get(m.UserID, projectID); err == nil {
		return domain.ActiveSession{}, ports.ErrActiveSessionConflict
	}
	a := domain.ActiveSession{
		UserID:    m.UserID,
		ProjectID: projectID,
		Tag:       tag,
		Note:      note,
		StartedAt: time.Now(),
	}
	return a, m.Active.Upsert(a)
}

// Stop ends the active session and creates a finished Session row.
func (m *FakeWorktimeMachine) Stop(projectID string) (domain.Session, error) {
	if m.StopErr != nil {
		return domain.Session{}, m.StopErr
	}
	a, err := m.Active.Get(m.UserID, projectID)
	if err != nil {
		return domain.Session{}, ports.ErrActiveSessionNotFound
	}
	_ = m.Active.Delete(m.UserID, projectID)
	now := time.Now()
	s := domain.Session{
		UserID:    m.UserID,
		ProjectID: projectID,
		Tag:       a.Tag,
		Note:      a.Note,
		Start:     a.StartedAt,
		Stop:      now,
		Elapsed:   now.Sub(a.StartedAt),
	}
	return s, m.Sessions.Upsert(s)
}

// Pause suspends the active session (marks PausedAt).
func (m *FakeWorktimeMachine) Pause(projectID string) (domain.ActiveSession, error) {
	a, err := m.Active.Get(m.UserID, projectID)
	if err != nil {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	now := time.Now()
	a.PausedAt = &now
	return a, m.Active.Upsert(a)
}

// Resume clears the PausedAt marker.
func (m *FakeWorktimeMachine) Resume(projectID string) (domain.ActiveSession, error) {
	a, err := m.Active.Get(m.UserID, projectID)
	if err != nil {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	a.PausedAt = nil
	return a, m.Active.Upsert(a)
}

// CorrectStart moves the start time of the active session.
func (m *FakeWorktimeMachine) CorrectStart(projectID string, startedAt time.Time) (domain.ActiveSession, error) {
	a, err := m.Active.Get(m.UserID, projectID)
	if err != nil {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	a.StartedAt = startedAt
	return a, m.Active.Upsert(a)
}
