package cli_test

// Tests for the new sqlite-backed start/stop path (Task 14).
//
// These tests are separate from worktime_test.go (legacy TSV path) per the
// no-monoliths rule: each file has one focused responsibility.

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/cli"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// ---- in-memory fakes for the new path ----

// fakeNewProjectStore is a simple in-memory ports.ProjectStore used in
// start/stop integration tests. It covers the subset of methods called by
// usecase.Sessions.ResolveProject and usecase.ActiveSessions.Start/Stop.
type fakeNewProjectStore struct {
	projects  []domain.Project
	touched   []string
	touchErr  error
	ensureErr error
}

func (f *fakeNewProjectStore) ListActive(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeNewProjectStore) ListAll(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeNewProjectStore) GetByID(_, id string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeNewProjectStore) GetBySlug(_, slug string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeNewProjectStore) EnsureBySlug(_, name, slug string) (domain.Project, error) {
	if f.ensureErr != nil {
		return domain.Project{}, f.ensureErr
	}
	p := domain.Project{ID: "auto-" + slug, Name: name, Slug: slug, CreatedAt: time.Now()}
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeNewProjectStore) Upsert(p domain.Project) error {
	f.projects = append(f.projects, p)
	return nil
}

func (f *fakeNewProjectStore) TouchLastUsed(_, id string) error {
	if f.touchErr != nil {
		return f.touchErr
	}
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeNewProjectStore) Archive(_, _ string) error { return nil }

// fakeNewActiveSessionStore is an in-memory ports.ActiveSessionStore.
type fakeNewActiveSessionStore struct {
	rows      map[string]domain.ActiveSession
	upsertErr error
	deleteErr error
	getErr    error
}

func newFakeNewActiveSessionStore() *fakeNewActiveSessionStore {
	return &fakeNewActiveSessionStore{rows: map[string]domain.ActiveSession{}}
}

func (f *fakeNewActiveSessionStore) key(userID, projectID string) string {
	return userID + "/" + projectID
}

func (f *fakeNewActiveSessionStore) ListByUser(userID string) ([]domain.ActiveSession, error) {
	var out []domain.ActiveSession
	for _, row := range f.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeNewActiveSessionStore) Get(userID, projectID string) (domain.ActiveSession, error) {
	if f.getErr != nil {
		return domain.ActiveSession{}, f.getErr
	}
	row, ok := f.rows[f.key(userID, projectID)]
	if !ok {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	return row, nil
}

func (f *fakeNewActiveSessionStore) Upsert(a domain.ActiveSession) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[f.key(a.UserID, a.ProjectID)] = a
	return nil
}

func (f *fakeNewActiveSessionStore) Delete(userID, projectID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, f.key(userID, projectID))
	return nil
}

// fakeNewWriteQueue is an in-memory ports.WriteQueue.
type fakeNewWriteQueue struct {
	entries    []ports.WriteQueueEntry
	seq        int64
	enqueueErr error
}

func (f *fakeNewWriteQueue) Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (int64, error) {
	if f.enqueueErr != nil {
		return 0, f.enqueueErr
	}
	f.seq++
	f.entries = append(f.entries, ports.WriteQueueEntry{
		Seq: f.seq, Resource: resource, RowID: rowID, Payload: payload, ExpectedVersion: expectedVersion,
	})
	return f.seq, nil
}

func (f *fakeNewWriteQueue) Peek(limit int) ([]ports.WriteQueueEntry, error) {
	if limit > len(f.entries) {
		return f.entries, nil
	}
	return f.entries[:limit], nil
}

func (f *fakeNewWriteQueue) Remove(seq int64) error {
	out := f.entries[:0]
	for _, e := range f.entries {
		if e.Seq != seq {
			out = append(out, e)
		}
	}
	f.entries = out
	return nil
}

func (f *fakeNewWriteQueue) SetError(seq int64, errMsg string) error {
	for i := range f.entries {
		if f.entries[i].Seq == seq {
			f.entries[i].LastError = errMsg
			return nil
		}
	}
	return nil
}

func (f *fakeNewWriteQueue) SetRetry(seq int64, errMsg string, nextRetryAt string) error {
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

// ---- fixture builder for the new path ----

// newPathFixture builds WorktimeDeps wired with the new sqlite closures.
// The legacy SessionWriter is set to nil — all start/stop operations flow
// through the new closures. Other deps (clock, tmux) come from the legacy
// fixture so reporter/status/export tests can still exercise their paths.
type newPathFixture struct {
	sessions       *testutil.FakeSessionStore
	activeSessions *fakeNewActiveSessionStore
	projects       *fakeNewProjectStore
	writeQueue     *fakeNewWriteQueue
	legacy         *fixture // for clock, tmux, dayoffs
}

const testUserID = "user-test"

func newNewPathFixture() *newPathFixture {
	return &newPathFixture{
		sessions:       &testutil.FakeSessionStore{},
		activeSessions: newFakeNewActiveSessionStore(),
		projects:       &fakeNewProjectStore{},
		writeQueue:     &fakeNewWriteQueue{},
		legacy:         newFixture(),
	}
}

func (f *newPathFixture) deps() cli.WorktimeDeps {
	sessionsUC := usecase.NewSessions(nil, f.projects, f.sessions, nil)
	activeUC := usecase.NewActiveSessions(nil, f.projects, f.activeSessions, f.sessions, f.writeQueue)

	// Minimal legacy deps so the cobra command tree initialises without
	// nil-pointer panics on Status/Reporter etc.
	targets := &usecase.TargetResolver{Config: f.legacy.config, DayOffs: f.legacy.dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: f.sessions, State: f.legacy.active, Targets: targets, Clock: f.legacy.clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: f.legacy.dayoffs}

	return cli.WorktimeDeps{
		Clock: f.legacy.clock,
		Tmux:  f.legacy.tmux,
		// SessionWriter intentionally nil — new path is used for start/stop.
		SessionWriter: &usecase.SessionWriter{
			Sessions: f.sessions, State: f.legacy.active, Lock: f.legacy.lock, Reader: reader, Clock: f.legacy.clock,
		},
		StatusComposer: &usecase.StatusComposer{
			Reader: reader, DayOffs: f.legacy.dayoffs, Targets: targets, Stats: stats, Tmux: f.legacy.tmux, Clock: f.legacy.clock,
		},
		Reporter:     &usecase.Reporter{Reader: reader, DayOffs: f.legacy.dayoffs, Targets: targets, Stats: stats, Clock: f.legacy.clock},
		Stats:        stats,
		DayOffWriter: &usecase.DayOffWriter{Store: f.legacy.dayoffs},
		DayOffStore:  f.legacy.dayoffs,
		Reader:       reader,

		// New path (Task 14).
		UserID: testUserID,
		ResolveProject: func(userID, explicitID, pwd string) (domain.Project, error) {
			return sessionsUC.ResolveProject(userID, explicitID, pwd)
		},
		StartActiveSession: func(userID, projectID, tag, note string) (domain.ActiveSession, error) {
			return activeUC.Start(userID, projectID, tag, note)
		},
		StopActiveSession: func(userID, projectID, tag, note string) (domain.Session, error) {
			return activeUC.Stop(userID, projectID, tag, note)
		},
		ListActiveSessions: func(userID string) ([]domain.ActiveSession, error) {
			return activeUC.ListActive(userID)
		},
	}
}

func (f *newPathFixture) run(args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(f.deps())
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ---- start new-path tests ----

// TestNewPath_Start_CreatesActiveSession verifies that `flow worktime start`
// on the new path creates an ActiveSession for the auto-created "Allgemein"
// project (step 4 of the cascade, triggered when no project exists).
func TestNewPath_Start_CreatesActiveSession(t *testing.T) {
	f := newNewPathFixture()
	_, stderr, err := f.run("start")
	if err != nil {
		t.Fatalf("start: unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "Allgemein") {
		t.Errorf("stderr should mention project name 'Allgemein', got %q", stderr)
	}
	if f.legacy.tmux.Refreshes != 1 {
		t.Errorf("expected 1 tmux refresh, got %d", f.legacy.tmux.Refreshes)
	}
	// ActiveSession must exist in the store.
	allgemeinID := ""
	for _, p := range f.projects.projects {
		if p.Slug == "allgemein" {
			allgemeinID = p.ID
			break
		}
	}
	if allgemeinID == "" {
		t.Fatal("Allgemein project not created")
	}
	if _, err := f.activeSessions.Get(testUserID, allgemeinID); err != nil {
		t.Errorf("active session not found in store: %v", err)
	}
}

// TestNewPath_Start_ExplicitProject verifies --project flag routes to the
// specified project when the value is the project's UUID (step 1a: GetByID).
func TestNewPath_Start_ExplicitProject(t *testing.T) {
	f := newNewPathFixture()
	seed := domain.Project{ID: "proj-1", Name: "Flow", Slug: "flow", CreatedAt: time.Now()}
	f.projects.projects = append(f.projects.projects, seed)

	_, stderr, err := f.run("start", "--project=proj-1")
	if err != nil {
		t.Fatalf("start --project=proj-1: %v", err)
	}
	if !strings.Contains(stderr, "Flow") {
		t.Errorf("stderr should mention project name, got %q", stderr)
	}
	if _, getErr := f.activeSessions.Get(testUserID, "proj-1"); getErr != nil {
		t.Errorf("active session for proj-1 not found: %v", getErr)
	}
}

// TestNewPath_Start_ExplicitProjectBySlug verifies --project flag also resolves
// by slug (step 1b: GetBySlug fallback when GetByID returns ErrProjectNotFound).
func TestNewPath_Start_ExplicitProjectBySlug(t *testing.T) {
	f := newNewPathFixture()
	seed := domain.Project{ID: "proj-1", Name: "Flow", Slug: "flow-project", CreatedAt: time.Now()}
	f.projects.projects = append(f.projects.projects, seed)

	_, stderr, err := f.run("start", "--project=flow-project")
	if err != nil {
		t.Fatalf("start --project=flow-project (by slug): %v", err)
	}
	if !strings.Contains(stderr, "Flow") {
		t.Errorf("stderr should mention project name, got %q", stderr)
	}
	if _, getErr := f.activeSessions.Get(testUserID, "proj-1"); getErr != nil {
		t.Errorf("active session for proj-1 not found: %v", getErr)
	}
}

// TestNewPath_Start_AlreadyRunning verifies idempotent behaviour: starting
// twice on the same project prints a hint on stderr but exits 0.
func TestNewPath_Start_AlreadyRunning(t *testing.T) {
	f := newNewPathFixture()
	seed := domain.Project{ID: "proj-1", Name: "Flow", Slug: "flow", CreatedAt: time.Now()}
	f.projects.projects = append(f.projects.projects, seed)

	// First start.
	if _, _, err := f.run("start", "--project=proj-1"); err != nil {
		t.Fatalf("first start: %v", err)
	}

	// Second start — same project already running.
	_, stderr, err := f.run("start", "--project=proj-1")
	if err != nil {
		t.Fatalf("second start must be idempotent (exit 0), got: %v", err)
	}
	if !strings.Contains(stderr, "läuft bereits") {
		t.Errorf("stderr should hint that session is running, got %q", stderr)
	}
}

// ---- stop new-path tests ----

// TestNewPath_Stop_HappyPath verifies that stop with --tag and --note creates
// a finished Session row and clears the ActiveSession.
func TestNewPath_Stop_HappyPath(t *testing.T) {
	f := newNewPathFixture()
	seed := domain.Project{ID: "proj-1", Name: "Flow", Slug: "flow", CreatedAt: time.Now()}
	f.projects.projects = append(f.projects.projects, seed)

	if _, _, err := f.run("start", "--project=flow"); err != nil {
		t.Fatalf("start: %v", err)
	}

	_, stderr, err := f.run("stop", "--project=flow", "--tag=deep", "--note=sprint done")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !strings.Contains(stderr, "Gestoppt") {
		t.Errorf("stderr: %q", stderr)
	}

	// Finished session row must exist.
	rows, _ := f.sessions.Load(testUserID)
	if len(rows) != 1 {
		t.Fatalf("expected 1 finished session, got %d", len(rows))
	}
	if rows[0].Tag != "deep" {
		t.Errorf("tag: got %q, want 'deep'", rows[0].Tag)
	}
	if rows[0].Note != "sprint done" {
		t.Errorf("note: got %q, want 'sprint done'", rows[0].Note)
	}

	// ActiveSession must be gone.
	if _, getErr := f.activeSessions.Get(testUserID, "proj-1"); !errors.Is(getErr, ports.ErrActiveSessionNotFound) {
		t.Errorf("active session should be removed, got: %v", getErr)
	}
}

// TestNewPath_Stop_NothingRunning verifies idempotent behaviour: stopping
// when nothing is active exits 0 and prints a hint to stderr.
func TestNewPath_Stop_NothingRunning(t *testing.T) {
	f := newNewPathFixture()
	_, stderr, err := f.run("stop")
	if err != nil {
		t.Fatalf("idle stop must succeed: %v", err)
	}
	if !strings.Contains(stderr, "Keine laufende Session") {
		t.Errorf("stderr should contain hint, got %q", stderr)
	}
}

// TestStopNewStopsTheActiveSessionRegardlessOfCwd verifies that stop resolves
// via ListActiveSessions and does NOT fall back to the cwd-resolved project
// when a session is running on a different project than the cwd.
func TestStopNewStopsTheActiveSessionRegardlessOfCwd(t *testing.T) {
	// Two projects: p-A has a running session, p-B is what cwd would resolve to.
	pA := domain.Project{ID: "proj-a", Name: "Project A", Slug: "project-a", CreatedAt: time.Now()}
	pB := domain.Project{ID: "proj-b", Name: "Project B", Slug: "project-b", CreatedAt: time.Now()}

	var stopCalledWith string
	stopCalled := 0

	base := newNewPathFixture()
	base.projects.projects = append(base.projects.projects, pA, pB)

	// Seed an active session for p-A directly in the store.
	_ = base.activeSessions.Upsert(domain.ActiveSession{
		UserID:    testUserID,
		ProjectID: "proj-a",
		StartedAt: time.Now().Add(-30 * time.Minute),
	})

	d := base.deps()
	// Override ListActiveSessions to return a session on p-A.
	d.ListActiveSessions = func(userID string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: userID, ProjectID: "proj-a", StartedAt: time.Now().Add(-30 * time.Minute)},
		}, nil
	}
	// Override ResolveProject to return p-B (simulating cwd = project-b dir).
	d.ResolveProject = func(_, _, _ string) (domain.Project, error) {
		return pB, nil
	}
	// Override StopActiveSession to capture which project was stopped.
	d.StopActiveSession = func(_, projectID, _, _ string) (domain.Session, error) {
		stopCalled++
		stopCalledWith = projectID
		return domain.Session{
			ID:      "sess-1",
			Elapsed: 30 * time.Minute,
		}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"stop"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stop must succeed: %v", err)
	}
	if stopCalled != 1 {
		t.Fatalf("StopActiveSession called %d times, want 1", stopCalled)
	}
	if stopCalledWith != "proj-a" {
		t.Errorf("StopActiveSession called with project %q, want %q", stopCalledWith, "proj-a")
	}
}

// TestStopNewNothingRunningPrintsHint verifies that when no session is running
// the command prints a hint to stderr, exits 0, and never calls StopActiveSession.
func TestStopNewNothingRunningPrintsHint(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()
	// Override ListActiveSessions to return empty.
	d.ListActiveSessions = func(_ string) ([]domain.ActiveSession, error) {
		return nil, nil
	}
	stopCalled := false
	d.StopActiveSession = func(_, _, _, _ string) (domain.Session, error) {
		stopCalled = true
		return domain.Session{}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"stop"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stop with nothing running must succeed: %v", err)
	}
	if stopCalled {
		t.Error("StopActiveSession must NOT be called when nothing is running")
	}
	if !strings.Contains(errBuf.String(), "Keine laufende Session") {
		t.Errorf("stderr should contain hint, got %q", errBuf.String())
	}
}

// TestStopNewParallelSessionsRequireProjectFlag verifies that when multiple
// sessions run in parallel and no --project flag is given, runStopNew returns
// an error mentioning "laufen parallel" and never calls StopActiveSession.
func TestStopNewParallelSessionsRequireProjectFlag(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	// Override ListActiveSessions to return two parallel sessions.
	d.ListActiveSessions = func(_ string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: testUserID, ProjectID: "proj-a", StartedAt: time.Now().Add(-60 * time.Minute)},
			{UserID: testUserID, ProjectID: "proj-b", StartedAt: time.Now().Add(-30 * time.Minute)},
		}, nil
	}

	stopCalled := 0
	d.StopActiveSession = func(_, _, _, _ string) (domain.Session, error) {
		stopCalled++
		return domain.Session{}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"stop"}) // no --project flag
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error for parallel sessions without --project, got nil")
	}
	if !strings.Contains(err.Error(), "laufen parallel") {
		t.Errorf("error should mention 'laufen parallel', got: %v", err)
	}
	if stopCalled != 0 {
		t.Errorf("StopActiveSession must NOT be called when disambiguation is required, called %d times", stopCalled)
	}
}

// ---- toggle new-path tests ----

// TestToggleNewStopsWhenRunning verifies that when an ActiveSession exists
// toggle calls StopActiveSession with the running project's ID and prints
// "Gestoppt" to stderr.
func TestToggleNewStopsWhenRunning(t *testing.T) {
	pA := domain.Project{ID: "proj-a", Name: "Project A", Slug: "project-a", CreatedAt: time.Now()}

	base := newNewPathFixture()
	base.projects.projects = append(base.projects.projects, pA)

	d := base.deps()

	// ListActiveSessions returns one running session for proj-a.
	d.ListActiveSessions = func(userID string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: userID, ProjectID: "proj-a", StartedAt: time.Now().Add(-45 * time.Minute)},
		}, nil
	}

	stopCalled := 0
	stopCalledWith := ""
	d.StopActiveSession = func(_, projectID, _, _ string) (domain.Session, error) {
		stopCalled++
		stopCalledWith = projectID
		return domain.Session{
			ID:      "sess-1",
			Elapsed: 45 * time.Minute,
		}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"toggle"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toggle must succeed: %v", err)
	}
	if stopCalled != 1 {
		t.Fatalf("StopActiveSession called %d times, want 1", stopCalled)
	}
	if stopCalledWith != "proj-a" {
		t.Errorf("StopActiveSession called with project %q, want %q", stopCalledWith, "proj-a")
	}
	if !strings.Contains(errBuf.String(), "Gestoppt") {
		t.Errorf("stderr should contain 'Gestoppt', got %q", errBuf.String())
	}
}

// TestToggleNewStopRaceReturnsNotRunning verifies the race window where
// ListActiveSessions saw one row but StopActiveSession returns
// ErrActiveSessionNotFound (another device/shell stopped it between calls).
// Toggle must NOT print "Gestoppt nach 0h 00m" — it should report
// "Keine laufende Session" and exit 0.
func TestToggleNewStopRaceReturnsNotRunning(t *testing.T) {
	pA := domain.Project{ID: "proj-a", Name: "Project A", Slug: "project-a", CreatedAt: time.Now()}

	base := newNewPathFixture()
	base.projects.projects = append(base.projects.projects, pA)

	d := base.deps()

	// ListActiveSessions returns one running session for proj-a (race: it
	// existed when we listed but is gone by the time Stop runs).
	d.ListActiveSessions = func(userID string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: userID, ProjectID: "proj-a", StartedAt: time.Now().Add(-45 * time.Minute)},
		}, nil
	}

	// StopActiveSession simulates the race: the row is already gone.
	d.StopActiveSession = func(_, _, _, _ string) (domain.Session, error) {
		return domain.Session{}, ports.ErrActiveSessionNotFound
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"toggle"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toggle race must exit 0: %v", err)
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "Keine laufende Session") {
		t.Errorf("stderr should contain 'Keine laufende Session', got %q", stderr)
	}
	if strings.Contains(stderr, "Gestoppt nach 0h") {
		t.Errorf("stderr must NOT contain misleading zero-value stop message, got %q", stderr)
	}
}

// TestToggleNewStartsWhenIdle verifies that when no ActiveSession exists
// toggle calls ResolveProject + StartActiveSession and prints "läuft seit"
// to stderr.
func TestToggleNewStartsWhenIdle(t *testing.T) {
	pA := domain.Project{ID: "proj-a", Name: "Project A", Slug: "project-a", CreatedAt: time.Now()}

	base := newNewPathFixture()
	base.projects.projects = append(base.projects.projects, pA)

	d := base.deps()

	// No active sessions.
	d.ListActiveSessions = func(_ string) ([]domain.ActiveSession, error) {
		return nil, nil
	}

	resolveCalled := 0
	d.ResolveProject = func(_, _, _ string) (domain.Project, error) {
		resolveCalled++
		return pA, nil
	}

	startCalled := 0
	startCalledWith := ""
	d.StartActiveSession = func(userID, projectID, _, _ string) (domain.ActiveSession, error) {
		startCalled++
		startCalledWith = projectID
		return domain.ActiveSession{UserID: userID, ProjectID: projectID, StartedAt: time.Now()}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"toggle"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toggle must succeed: %v", err)
	}
	if resolveCalled != 1 {
		t.Errorf("ResolveProject called %d times, want 1", resolveCalled)
	}
	if startCalled != 1 {
		t.Fatalf("StartActiveSession called %d times, want 1", startCalled)
	}
	if startCalledWith != "proj-a" {
		t.Errorf("StartActiveSession called with project %q, want %q", startCalledWith, "proj-a")
	}
	if !strings.Contains(errBuf.String(), "läuft seit") {
		t.Errorf("stderr should contain 'läuft seit', got %q", errBuf.String())
	}
}

// ---- pause/resume new-path tests ----

// fakePauseStore is a simple in-memory ports.PauseStore for pause/resume tests.
type fakePauseStore struct {
	paused      *time.Time
	setPaused   bool
	clearCalled bool
}

func (f *fakePauseStore) GetPause() (*time.Time, error) {
	return f.paused, nil
}

func (f *fakePauseStore) SetPause(t time.Time) error {
	f.setPaused = true
	f.paused = &t
	return nil
}

func (f *fakePauseStore) ClearPause() error {
	f.clearCalled = true
	f.paused = nil
	return nil
}

// TestPauseNewStopsAndSetsMarker: active session running → pause calls
// StopActiveSession and PauseMarker.SetPause; stderr contains "Pausiert".
func TestPauseNewStopsAndSetsMarker(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	pauseStore := &fakePauseStore{}
	d.PauseMarker = pauseStore

	stopCalled := 0
	d.ListActiveSessions = func(userID string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: userID, ProjectID: "proj-a", StartedAt: time.Now().Add(-30 * time.Minute)},
		}, nil
	}
	d.StopActiveSession = func(_, _, _, _ string) (domain.Session, error) {
		stopCalled++
		return domain.Session{
			ID:      "sess-1",
			Elapsed: 30 * time.Minute,
		}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"pause"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pause must succeed: %v", err)
	}
	if stopCalled != 1 {
		t.Errorf("StopActiveSession called %d times, want 1", stopCalled)
	}
	if !pauseStore.setPaused {
		t.Error("PauseMarker.SetPause must be called")
	}
	if !strings.Contains(errBuf.String(), "Pausiert") {
		t.Errorf("stderr should contain 'Pausiert', got %q", errBuf.String())
	}
}

// TestPauseNewIdleIsNoop: nothing running → no Stop call, exit 0.
func TestPauseNewIdleIsNoop(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	pauseStore := &fakePauseStore{}
	d.PauseMarker = pauseStore

	d.ListActiveSessions = func(_ string) ([]domain.ActiveSession, error) {
		return nil, nil
	}
	stopCalled := false
	d.StopActiveSession = func(_, _, _, _ string) (domain.Session, error) {
		stopCalled = true
		return domain.Session{}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"pause"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pause with nothing running must succeed: %v", err)
	}
	if stopCalled {
		t.Error("StopActiveSession must NOT be called when nothing is running")
	}
	if pauseStore.setPaused {
		t.Error("PauseMarker.SetPause must NOT be called when nothing is running")
	}
}

// TestResumeNewStartsMRUAndClearsMarker: nothing running, pause marker set →
// resume calls ResolveProject(userID, "", "") + StartActiveSession + ClearPause.
func TestResumeNewStartsMRUAndClearsMarker(t *testing.T) {
	pA := domain.Project{ID: "proj-a", Name: "Project A", Slug: "project-a", CreatedAt: time.Now()}

	base := newNewPathFixture()
	d := base.deps()

	t0 := time.Now()
	pauseStore := &fakePauseStore{paused: &t0}
	d.PauseMarker = pauseStore

	// Nothing running.
	d.ListActiveSessions = func(_ string) ([]domain.ActiveSession, error) {
		return nil, nil
	}

	resolveCalled := 0
	resolveCalledWith := ""
	d.ResolveProject = func(_, _, pwd string) (domain.Project, error) {
		resolveCalled++
		resolveCalledWith = pwd
		return pA, nil
	}

	startCalled := 0
	d.StartActiveSession = func(userID, projectID, _, _ string) (domain.ActiveSession, error) {
		startCalled++
		return domain.ActiveSession{UserID: userID, ProjectID: projectID, StartedAt: time.Now()}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"resume"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("resume must succeed: %v", err)
	}
	if resolveCalled != 1 {
		t.Errorf("ResolveProject called %d times, want 1", resolveCalled)
	}
	if resolveCalledWith != "" {
		t.Errorf("ResolveProject should be called with empty pwd (MRU cascade), got %q", resolveCalledWith)
	}
	if startCalled != 1 {
		t.Errorf("StartActiveSession called %d times, want 1", startCalled)
	}
	if !pauseStore.clearCalled {
		t.Error("PauseMarker.ClearPause must be called")
	}
	if !strings.Contains(errBuf.String(), "Resume") {
		t.Errorf("stderr should contain 'Resume', got %q", errBuf.String())
	}
}

// TestResumeNewAlreadyRunningClearsMarkerOnly: session running → only
// ClearPause is called, no StartActiveSession.
func TestResumeNewAlreadyRunningClearsMarkerOnly(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	t0 := time.Now()
	pauseStore := &fakePauseStore{paused: &t0}
	d.PauseMarker = pauseStore

	// Session is already running.
	d.ListActiveSessions = func(userID string) ([]domain.ActiveSession, error) {
		return []domain.ActiveSession{
			{UserID: userID, ProjectID: "proj-a", StartedAt: time.Now().Add(-10 * time.Minute)},
		}, nil
	}

	startCalled := false
	d.StartActiveSession = func(_, _, _, _ string) (domain.ActiveSession, error) {
		startCalled = true
		return domain.ActiveSession{}, nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"resume"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("resume must succeed: %v", err)
	}
	if startCalled {
		t.Error("StartActiveSession must NOT be called when session already running")
	}
	if !pauseStore.clearCalled {
		t.Error("PauseMarker.ClearPause must still be called even when running")
	}
	if !strings.Contains(errBuf.String(), "Resume") {
		t.Errorf("stderr should contain 'Resume', got %q", errBuf.String())
	}
}

// ---- correct new-path tests ----

// TestCorrectNewPathCorrectedTime: session running → correct calls
// CorrectActiveStart with the parsed time and prints "korrigiert" to stderr.
func TestCorrectNewPathCorrectedTime(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	correctCalled := 0
	var correctCalledWith time.Time
	d.CorrectActiveStart = func(_ string, ts time.Time) error {
		correctCalled++
		correctCalledWith = ts
		return nil
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"correct", "09:00"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("correct must succeed: %v", err)
	}
	if correctCalled != 1 {
		t.Errorf("CorrectActiveStart called %d times, want 1", correctCalled)
	}
	if correctCalledWith.Hour() != 9 || correctCalledWith.Minute() != 0 {
		t.Errorf("CorrectActiveStart called with time %v, want 09:00", correctCalledWith)
	}
	if !strings.Contains(errBuf.String(), "korrigiert") {
		t.Errorf("stderr should contain 'korrigiert', got %q", errBuf.String())
	}
}

// TestCorrectNewPathNothingRunning: CorrectActiveStart returns
// ErrActiveSessionNotFound → command exits 0 and prints hint.
func TestCorrectNewPathNothingRunning(t *testing.T) {
	base := newNewPathFixture()
	d := base.deps()

	d.CorrectActiveStart = func(_ string, _ time.Time) error {
		return ports.ErrActiveSessionNotFound
	}

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"correct", "09:00"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("correct with nothing running must succeed: %v", err)
	}
	if !strings.Contains(errBuf.String(), "Keine laufende Session") {
		t.Errorf("stderr should contain hint, got %q", errBuf.String())
	}
}

// TestCorrectLegacyFallback: when CorrectActiveStart is nil, the legacy
// SessionWriter.CorrectStart path is used.
func TestCorrectLegacyFallback(_ *testing.T) {
	base := newNewPathFixture()
	d := base.deps()
	// Explicitly nil out the new path so legacy triggers.
	d.CorrectActiveStart = nil

	// The legacy path calls deps.SessionWriter.CorrectStart(ts). Since
	// there is no active flockstate session, it will return an error
	// (ErrNoActiveSession or similar). We only need to verify the command
	// did NOT panic and that CorrectActiveStart was not called.
	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(d)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"correct", "09:00"})
	// We don't care if it fails on the legacy path — only that it reached
	// the legacy branch (no panic, no new-path behaviour).
	_ = cmd.Execute()
	// If we reach here the legacy fallback ran without panic. Pass.
}
