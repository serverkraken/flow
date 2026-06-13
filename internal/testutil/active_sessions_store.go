package testutil

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.ActiveSessionStore = (*FakeActiveSessionStoreV2)(nil)

// FakeActiveSessionStoreV2 is an in-memory ports.ActiveSessionStore for the
// new multi-project ActiveSessions use case. Named V2 to avoid a collision
// with the existing FakeActiveSessionStore (ports.LegacyActiveStore).
type FakeActiveSessionStoreV2 struct {
	// Rows holds the current active sessions keyed by "userID|projectID".
	Rows map[string]domain.ActiveSession
	Err  error
}

func (f *FakeActiveSessionStoreV2) key(userID, projectID string) string {
	return userID + "|" + projectID
}

// ListByUser implements ports.ActiveSessionStore.
func (f *FakeActiveSessionStoreV2) ListByUser(userID string) ([]domain.ActiveSession, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var out []domain.ActiveSession
	for _, as := range f.Rows {
		if as.UserID == userID {
			out = append(out, as)
		}
	}
	return out, nil
}

// Get implements ports.ActiveSessionStore.
func (f *FakeActiveSessionStoreV2) Get(userID, projectID string) (domain.ActiveSession, error) {
	if f.Err != nil {
		return domain.ActiveSession{}, f.Err
	}
	as, ok := f.Rows[f.key(userID, projectID)]
	if !ok {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	return as, nil
}

// Upsert implements ports.ActiveSessionStore.
func (f *FakeActiveSessionStoreV2) Upsert(a domain.ActiveSession) error {
	if f.Err != nil {
		return f.Err
	}
	if f.Rows == nil {
		f.Rows = map[string]domain.ActiveSession{}
	}
	f.Rows[f.key(a.UserID, a.ProjectID)] = a
	return nil
}

// Delete implements ports.ActiveSessionStore.
func (f *FakeActiveSessionStoreV2) Delete(userID, projectID string) error {
	if f.Err != nil {
		return f.Err
	}
	delete(f.Rows, f.key(userID, projectID))
	return nil
}
