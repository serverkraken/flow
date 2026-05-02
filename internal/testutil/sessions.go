package testutil

import "github.com/serverkraken/flow/internal/domain"

// FakeSessionStore is an in-memory ports.SessionStore. The slice is
// returned by reference from LoadAll, so tests can both seed pre-existing
// sessions and observe what the use-case wrote.
type FakeSessionStore struct {
	Sessions []domain.Session
	Err      error // returned by every method when non-nil
}

func (f *FakeSessionStore) LoadAll() ([]domain.Session, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]domain.Session, len(f.Sessions))
	copy(out, f.Sessions)
	return out, nil
}

func (f *FakeSessionStore) LoadFiltered(keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := f.LoadAll()
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

func (f *FakeSessionStore) Append(s domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

func (f *FakeSessionStore) Rewrite(sessions []domain.Session) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions = make([]domain.Session, len(sessions))
	copy(f.Sessions, sessions)
	return nil
}
