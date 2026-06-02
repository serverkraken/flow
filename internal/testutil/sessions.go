package testutil

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.SessionStore = (*FakeSessionStore)(nil)

// FakeSessionStore is an in-memory ports.SessionStore. The slice is
// returned by reference from Load/LoadAllLegacy, so tests can both seed
// pre-existing sessions and observe what the use-case wrote.
type FakeSessionStore struct {
	Sessions []domain.Session
	Err      error // returned by every method when non-nil
}

// LoadAllLegacy returns all sessions; helper for legacy-style test assertions.
func (f *FakeSessionStore) LoadAllLegacy() ([]domain.Session, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]domain.Session, len(f.Sessions))
	copy(out, f.Sessions)
	return out, nil
}

// LoadFilteredLegacy is the legacy 1-arg variant; kept for existing test
// assertions until Task 19.
func (f *FakeSessionStore) LoadFilteredLegacy(keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := f.LoadAllLegacy()
	if err != nil {
		return nil, err
	}
	out := make([]domain.Session, 0, len(all))
	for _, s := range all {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// Load implements ports.SessionStore.
func (f *FakeSessionStore) Load(_ string) ([]domain.Session, error) {
	return f.LoadAllLegacy()
}

// LoadFiltered implements ports.SessionStore.
func (f *FakeSessionStore) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	return f.LoadFilteredLegacy(keep)
}

// Upsert implements ports.SessionStore. Matches by (Date, Start) when ID
// is empty (legacy rows), otherwise by ID.
func (f *FakeSessionStore) Upsert(s domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	for i := range f.Sessions {
		if s.ID != "" && f.Sessions[i].ID == s.ID {
			f.Sessions[i] = s
			return nil
		}
		if s.ID == "" && f.Sessions[i].Date.Equal(s.Date) && f.Sessions[i].Start.Equal(s.Start) {
			f.Sessions[i] = s
			return nil
		}
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

// UpsertBatch implements ports.SessionStore.
func (f *FakeSessionStore) UpsertBatch(sessions []domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	for _, s := range sessions {
		if err := f.Upsert(s); err != nil {
			return err
		}
	}
	return nil
}

// Delete implements ports.SessionStore.
func (f *FakeSessionStore) Delete(_ string, id string) error {
	if f.Err != nil {
		return f.Err
	}
	out := f.Sessions[:0]
	for _, s := range f.Sessions {
		if s.ID != id {
			out = append(out, s)
		}
	}
	f.Sessions = out
	return nil
}

// — legacy helpers kept for callers that have not yet migrated —

// Append adds a single session. Used by existing tests and usecases that
// have not yet migrated to Upsert.
func (f *FakeSessionStore) Append(s domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

// AppendBatch appends multiple sessions. Used by existing tests and
// usecases that have not yet migrated to UpsertBatch.
func (f *FakeSessionStore) AppendBatch(sessions []domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions = append(f.Sessions, sessions...)
	return nil
}

// Rewrite replaces the entire session list atomically.
func (f *FakeSessionStore) Rewrite(sessions []domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions = make([]domain.Session, len(sessions))
	copy(f.Sessions, sessions)
	return nil
}
