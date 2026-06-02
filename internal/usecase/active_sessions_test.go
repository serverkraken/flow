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

// fakeASSessionStore is a minimal ports.SessionStore for ActiveSessions tests.
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

// ---- helpers ----

func mkActiveSessions(
	active *fakeActiveSessionStore,
	projects *fakeASProjectStore,
	sessions *fakeASSessionStore,
	queue *fakeWriteQueue,
) *usecase.ActiveSessions {
	return usecase.NewActiveSessions(nil, projects, active, sessions, queue)
}

// ---- Start tests ----

// Start happy-path: ActiveSession row appears in store, Project.TouchLastUsed
// called once, queue has one entry with resource="active_sessions" and
// expectedVersion=0.
func TestUnit_ActiveSessions_Start_HappyPath(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	projects := &fakeASProjectStore{}
	sessions := &fakeASSessionStore{}
	queue := &fakeWriteQueue{}

	uc := mkActiveSessions(active, projects, sessions, queue)

	before := time.Now().UTC()
	row, err := uc.Start("u1", "p1")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	// Row fields correct.
	if row.UserID != "u1" {
		t.Errorf("UserID: got %q, want %q", row.UserID, "u1")
	}
	if row.ProjectID != "p1" {
		t.Errorf("ProjectID: got %q, want %q", row.ProjectID, "p1")
	}
	if row.StartedAt.IsZero() {
		t.Error("StartedAt is zero")
	}
	if row.StartedAt.Before(before) || row.StartedAt.After(after) {
		t.Errorf("StartedAt %v outside [%v, %v]", row.StartedAt, before, after)
	}
	if row.StartedOnDevice == "" {
		t.Error("StartedOnDevice is empty")
	}

	// Upsert called once with correct row.
	if len(active.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(active.upserted))
	}
	stored := active.upserted[0]
	if stored.UserID != "u1" || stored.ProjectID != "p1" {
		t.Errorf("stored row mismatch: %+v", stored)
	}

	// TouchLastUsed called once for projectID.
	if len(projects.touched) != 1 || projects.touched[0] != "p1" {
		t.Errorf("TouchLastUsed: got %v, want [p1]", projects.touched)
	}

	// Queue has one entry.
	if len(queue.entries) != 1 {
		t.Fatalf("queue entries: got %d, want 1", len(queue.entries))
	}
	e := queue.entries[0]
	if e.Resource != "active_sessions" {
		t.Errorf("queue resource: got %q, want %q", e.Resource, "active_sessions")
	}
	if e.RowID != "p1" {
		t.Errorf("queue rowID: got %q, want %q", e.RowID, "p1")
	}
	if e.ExpectedVersion != 0 {
		t.Errorf("queue expectedVersion: got %d, want 0", e.ExpectedVersion)
	}
	if len(e.Payload) == 0 {
		t.Error("queue payload is empty")
	}
}

// Start when already-running returns ErrActiveSessionExists;
// no writes happen.
func TestUnit_ActiveSessions_Start_AlreadyRunning(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	// Pre-seed an existing active session.
	existing := domain.ActiveSession{
		UserID:    "u1",
		ProjectID: "p1",
		StartedAt: time.Now().UTC().Add(-10 * time.Minute),
	}
	_ = active.Upsert(existing)
	// Reset upserted tracking after the seed.
	active.upserted = nil

	projects := &fakeASProjectStore{}
	sessions := &fakeASSessionStore{}
	queue := &fakeWriteQueue{}

	uc := mkActiveSessions(active, projects, sessions, queue)

	_, err := uc.Start("u1", "p1")
	if !errors.Is(err, usecase.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists, got %v", err)
	}

	// No writes.
	if len(active.upserted) != 0 {
		t.Errorf("expected no upserts, got %d", len(active.upserted))
	}
	if len(projects.touched) != 0 {
		t.Errorf("expected no TouchLastUsed, got %d", len(projects.touched))
	}
	if len(queue.entries) != 0 {
		t.Errorf("expected no queue entries, got %d", len(queue.entries))
	}
}

// Start when active.Get returns an unexpected error (not ErrActiveSessionNotFound)
// → error is bubbled up.
func TestUnit_ActiveSessions_Start_GetUnexpectedError(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	unexpectedErr := errors.New("disk error")
	active.getErr = unexpectedErr

	uc := mkActiveSessions(active, &fakeASProjectStore{}, &fakeASSessionStore{}, &fakeWriteQueue{})

	_, err := uc.Start("u1", "p1")
	if !errors.Is(err, unexpectedErr) {
		t.Fatalf("expected disk error to bubble, got %v", err)
	}
}

// ---- Stop tests ----

// Stop happy-path: Session row created with correct fields, ActiveSession
// deleted, two queue entries (sessions + active_sessions_stop).
func TestUnit_ActiveSessions_Stop_HappyPath(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	startedAt := time.Now().UTC().Add(-30 * time.Minute)
	existing := domain.ActiveSession{
		UserID:    "u1",
		ProjectID: "p1",
		StartedAt: startedAt,
		Version:   3,
	}
	_ = active.Upsert(existing)
	active.upserted = nil // reset tracking

	sessions := &fakeASSessionStore{}
	projects := &fakeASProjectStore{}
	queue := &fakeWriteQueue{}

	uc := mkActiveSessions(active, projects, sessions, queue)

	before := time.Now().UTC()
	sess, err := uc.Stop("u1", "p1", "deep", "finished sprint")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}

	// Session fields.
	if sess.ID == "" {
		t.Error("Session ID is empty")
	}
	if sess.UserID != "u1" {
		t.Errorf("UserID: got %q, want %q", sess.UserID, "u1")
	}
	if sess.ProjectID != "p1" {
		t.Errorf("ProjectID: got %q, want %q", sess.ProjectID, "p1")
	}
	if sess.Start != startedAt {
		t.Errorf("Start: got %v, want %v", sess.Start, startedAt)
	}
	if sess.Stop.Before(before) || sess.Stop.After(after) {
		t.Errorf("Stop %v outside [%v, %v]", sess.Stop, before, after)
	}
	if sess.Tag != "deep" {
		t.Errorf("Tag: got %q, want %q", sess.Tag, "deep")
	}
	if sess.Note != "finished sprint" {
		t.Errorf("Note: got %q, want %q", sess.Note, "finished sprint")
	}
	expectedElapsed := sess.Stop.Sub(startedAt)
	if sess.Elapsed != expectedElapsed {
		t.Errorf("Elapsed: got %v, want %v", sess.Elapsed, expectedElapsed)
	}

	// Session.Upsert called.
	if len(sessions.upserted) != 1 {
		t.Fatalf("expected 1 session upsert, got %d", len(sessions.upserted))
	}

	// ActiveSession deleted.
	if len(active.deleted) != 1 {
		t.Fatalf("expected 1 active delete, got %d", len(active.deleted))
	}
	if active.deleted[0][0] != "u1" || active.deleted[0][1] != "p1" {
		t.Errorf("deleted key: got %v, want [u1 p1]", active.deleted[0])
	}

	// Two queue entries.
	if len(queue.entries) != 2 {
		t.Fatalf("expected 2 queue entries, got %d", len(queue.entries))
	}
	// First entry: sessions.
	e0 := queue.entries[0]
	if e0.Resource != "sessions" {
		t.Errorf("queue[0] resource: got %q, want %q", e0.Resource, "sessions")
	}
	if e0.RowID != sess.ID {
		t.Errorf("queue[0] rowID: got %q, want %q", e0.RowID, sess.ID)
	}
	// Second entry: active_sessions_stop with version from existing row.
	e1 := queue.entries[1]
	if e1.Resource != "active_sessions_stop" {
		t.Errorf("queue[1] resource: got %q, want %q", e1.Resource, "active_sessions_stop")
	}
	if e1.RowID != "p1" {
		t.Errorf("queue[1] rowID: got %q, want %q", e1.RowID, "p1")
	}
	if e1.ExpectedVersion != 3 {
		t.Errorf("queue[1] expectedVersion: got %d, want 3", e1.ExpectedVersion)
	}
}

// Stop when no active session exists: Get returns ErrActiveSessionNotFound
// → method bubbles the error; no session Upsert happens.
func TestUnit_ActiveSessions_Stop_NoActiveSession(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore() // empty
	sessions := &fakeASSessionStore{}
	queue := &fakeWriteQueue{}

	uc := mkActiveSessions(active, &fakeASProjectStore{}, sessions, queue)

	_, err := uc.Stop("u1", "p1", "", "")
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Fatalf("expected ErrActiveSessionNotFound, got %v", err)
	}
	if len(sessions.upserted) != 0 {
		t.Errorf("expected no session upserts, got %d", len(sessions.upserted))
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

	uc := mkActiveSessions(active, &fakeASProjectStore{}, &fakeASSessionStore{}, &fakeWriteQueue{})

	rows, err := uc.ListActive("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(rows))
	}
}

// ---- ForceTakeover tests ----

// ForceTakeover queues a start with the provided currentServerVersion (not 0).
func TestUnit_ActiveSessions_ForceTakeover_UsesServerVersion(t *testing.T) {
	t.Parallel()
	active := newFakeActiveSessionStore()
	queue := &fakeWriteQueue{}

	uc := mkActiveSessions(active, &fakeASProjectStore{}, &fakeASSessionStore{}, queue)

	err := uc.ForceTakeover("u1", "p1", 7)
	if err != nil {
		t.Fatalf("ForceTakeover: unexpected error: %v", err)
	}

	// ActiveSession upserted.
	if len(active.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(active.upserted))
	}

	// Queue entry with version=7.
	if len(queue.entries) != 1 {
		t.Fatalf("expected 1 queue entry, got %d", len(queue.entries))
	}
	e := queue.entries[0]
	if e.Resource != "active_sessions" {
		t.Errorf("queue resource: got %q, want %q", e.Resource, "active_sessions")
	}
	if e.ExpectedVersion != 7 {
		t.Errorf("queue expectedVersion: got %d, want 7", e.ExpectedVersion)
	}
}
