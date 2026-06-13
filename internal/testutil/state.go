package testutil

import (
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

var (
	_ ports.LegacyActiveStore = (*FakeActiveSessionStore)(nil)
	_ ports.Lock              = (*FakeLock)(nil)
)

// FakeActiveSessionStore is an in-memory ports.LegacyActiveStore. The
// nil-vs-set semantics of GetActive/GetPause are preserved.
type FakeActiveSessionStore struct {
	Active *time.Time
	Pause  *time.Time
	Err    error // returned by every method when non-nil
}

func (f *FakeActiveSessionStore) GetActive() (*time.Time, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Active, nil
}

func (f *FakeActiveSessionStore) SetActive(t time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	v := t
	f.Active = &v
	return nil
}

func (f *FakeActiveSessionStore) ClearActive() error {
	if f.Err != nil {
		return f.Err
	}
	f.Active = nil
	return nil
}

func (f *FakeActiveSessionStore) GetPause() (*time.Time, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Pause, nil
}

func (f *FakeActiveSessionStore) SetPause(t time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	v := t
	f.Pause = &v
	return nil
}

func (f *FakeActiveSessionStore) ClearPause() error {
	if f.Err != nil {
		return f.Err
	}
	f.Pause = nil
	return nil
}

// FakeLock is a no-op ports.Lock — fn runs immediately. Tests that exercise
// lock contention should write a custom implementation.
type FakeLock struct {
	// Calls counts how often With was invoked, for assertions.
	Calls int
	// Err is returned by With (and fn is not invoked) when non-nil.
	Err error
}

func (f *FakeLock) With(fn func() error) error {
	f.Calls++
	if f.Err != nil {
		return f.Err
	}
	return fn()
}
