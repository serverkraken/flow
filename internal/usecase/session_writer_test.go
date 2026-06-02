package usecase_test

// Shared helpers for the SessionWriter test suite. The 45 test
// functions live in three sibling files split by concern:
//   - session_writer_lifecycle_test.go (Start / Stop / Pause / Resume / Toggle / CorrectStart)
//   - session_writer_manual_test.go    (AddManual / Edit / Delete / SetTag / SetNote)
//   - session_writer_errors_test.go    (every error-propagation path)
// The flaky*Store types below drive the error-injection tests; the
// healthy in-memory equivalents live in internal/testutil.

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkWriter(now time.Time, opts ...readerOpt) *usecase.SessionWriter {
	reader := mkReader(now, nil, opts...)
	return &usecase.SessionWriter{
		Sessions: reader.Sessions,
		State:    reader.State,
		Lock:     &testutil.FakeLock{},
		Reader:   reader,
		Clock:    reader.Clock,
	}
}

// flakySessionStore fails only on the named method; other methods succeed
// against the in-memory Sessions slice.
type flakySessionStore struct {
	Sessions []domain.Session
	FailOn   string
}

// New ports.SessionStore interface methods (Task 3 shim).

func (f *flakySessionStore) Load(_ string) ([]domain.Session, error) {
	if f.FailOn == "LoadAll" {
		return nil, errors.New("boom")
	}
	out := make([]domain.Session, len(f.Sessions))
	copy(out, f.Sessions)
	return out, nil
}

func (f *flakySessionStore) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	if f.FailOn == "LoadFiltered" {
		return nil, errors.New("boom")
	}
	out := []domain.Session{}
	for _, s := range f.Sessions {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *flakySessionStore) Upsert(s domain.Session) error {
	if f.FailOn == "Upsert" || f.FailOn == "Append" {
		return errors.New("boom")
	}
	for i := range f.Sessions {
		if f.Sessions[i].Date.Equal(s.Date) && f.Sessions[i].Start.Equal(s.Start) {
			f.Sessions[i] = s
			return nil
		}
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

func (f *flakySessionStore) UpsertBatch(sessions []domain.Session) error {
	if f.FailOn == "AppendBatch" || f.FailOn == "Append" {
		// Treat the legacy "Append" knob as covering UpsertBatch too —
		// existing TestSessionWriter_*_AppendErr cases continue to assert
		// the failure surfaces from the use case.
		return errors.New("boom")
	}
	f.Sessions = append(f.Sessions, sessions...)
	return nil
}

func (f *flakySessionStore) Delete(_ string, id string) error {
	if f.FailOn == "Delete" {
		return errors.New("boom")
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

// Deprecated legacy methods — kept to satisfy the transitional interface
// until Task 19 removes the tsvsessions adapter.

func (f *flakySessionStore) Append(s domain.Session) error {
	if f.FailOn == "Append" {
		return errors.New("boom")
	}
	f.Sessions = append(f.Sessions, s)
	return nil
}

func (f *flakySessionStore) AppendBatch(sessions []domain.Session) error {
	if f.FailOn == "AppendBatch" || f.FailOn == "Append" {
		return errors.New("boom")
	}
	f.Sessions = append(f.Sessions, sessions...)
	return nil
}

func (f *flakySessionStore) Rewrite(sessions []domain.Session) error {
	if f.FailOn == "Rewrite" {
		return errors.New("boom")
	}
	f.Sessions = make([]domain.Session, len(sessions))
	copy(f.Sessions, sessions)
	return nil
}

// flakyActiveStore fails only on the named method.
type flakyActiveStore struct {
	Active *time.Time
	Pause  *time.Time
	FailOn string
}

func (f *flakyActiveStore) GetActive() (*time.Time, error) {
	if f.FailOn == "GetActive" {
		return nil, errors.New("boom")
	}
	return f.Active, nil
}

func (f *flakyActiveStore) SetActive(t time.Time) error {
	if f.FailOn == "SetActive" {
		return errors.New("boom")
	}
	v := t
	f.Active = &v
	return nil
}

func (f *flakyActiveStore) ClearActive() error {
	if f.FailOn == "ClearActive" {
		return errors.New("boom")
	}
	f.Active = nil
	return nil
}

func (f *flakyActiveStore) GetPause() (*time.Time, error) {
	if f.FailOn == "GetPause" {
		return nil, errors.New("boom")
	}
	return f.Pause, nil
}

func (f *flakyActiveStore) SetPause(t time.Time) error {
	if f.FailOn == "SetPause" {
		return errors.New("boom")
	}
	v := t
	f.Pause = &v
	return nil
}

func (f *flakyActiveStore) ClearPause() error {
	if f.FailOn == "ClearPause" {
		return errors.New("boom")
	}
	f.Pause = nil
	return nil
}
