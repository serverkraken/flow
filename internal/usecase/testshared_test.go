package usecase_test

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// fakeASSessionStore is a minimal ports.SessionStore for tests that wire
// ActiveSessions or Sessions use cases. Shared across mcp_tools_test and
// any other test that needs a basic session store without disk I/O.
type fakeASSessionStore struct {
	sessions  []domain.Session
	upserted  []domain.Session
	upsertErr error
}

func (f *fakeASSessionStore) Load(_ string) ([]domain.Session, error) {
	out := make([]domain.Session, len(f.sessions))
	copy(out, f.sessions)
	return out, nil
}

func (f *fakeASSessionStore) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	var out []domain.Session
	for _, s := range f.sessions {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeASSessionStore) Upsert(s domain.Session) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append(f.upserted, s)
	f.sessions = append(f.sessions, s)
	return nil
}

func (f *fakeASSessionStore) UpsertBatch(sessions []domain.Session) error {
	for _, s := range sessions {
		if err := f.Upsert(s); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeASSessionStore) Delete(_ string, id string) error {
	out := f.sessions[:0]
	for _, s := range f.sessions {
		if s.ID != id {
			out = append(out, s)
		}
	}
	f.sessions = out
	return nil
}

func (f *fakeASSessionStore) Append(s domain.Session) error {
	f.sessions = append(f.sessions, s)
	return nil
}

func (f *fakeASSessionStore) AppendBatch(sessions []domain.Session) error {
	f.sessions = append(f.sessions, sessions...)
	return nil
}

func (f *fakeASSessionStore) Rewrite(sessions []domain.Session) error {
	f.sessions = make([]domain.Session, len(sessions))
	copy(f.sessions, sessions)
	return nil
}

// fakeWriteQueue implements ports.WriteQueue in memory.
// Shared across active_sessions_test, repo_notes_test, mcp_tools_test.
type fakeWriteQueue struct {
	entries    []ports.WriteQueueEntry
	seq        int64
	enqueueErr error
}

func (f *fakeWriteQueue) Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (int64, error) {
	if f.enqueueErr != nil {
		return 0, f.enqueueErr
	}
	f.seq++
	f.entries = append(f.entries, ports.WriteQueueEntry{
		Seq:             f.seq,
		Resource:        resource,
		RowID:           rowID,
		Payload:         payload,
		ExpectedVersion: expectedVersion,
	})
	return f.seq, nil
}

func (f *fakeWriteQueue) Peek(limit int) ([]ports.WriteQueueEntry, error) {
	if limit > len(f.entries) {
		return f.entries, nil
	}
	return f.entries[:limit], nil
}

func (f *fakeWriteQueue) Remove(seq int64) error {
	out := f.entries[:0]
	for _, e := range f.entries {
		if e.Seq != seq {
			out = append(out, e)
		}
	}
	f.entries = out
	return nil
}

func (f *fakeWriteQueue) SetError(seq int64, errMsg string) error {
	for i := range f.entries {
		if f.entries[i].Seq == seq {
			f.entries[i].LastError = errMsg
			return nil
		}
	}
	return nil
}

func (f *fakeWriteQueue) SetRetry(seq int64, errMsg string, nextRetryAt string) error {
	for i := range f.entries {
		if f.entries[i].Seq == seq {
			f.entries[i].LastError = errMsg
			f.entries[i].Attempt++
			f.entries[i].NextRetryAt = nextRetryAt
			return nil
		}
	}
	return nil
}
