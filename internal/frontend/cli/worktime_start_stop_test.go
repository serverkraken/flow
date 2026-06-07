package cli_test

// Tests for the new sqlite-backed start/stop path (Task 14) and the TSV
// migration guard wired as PersistentPreRunE on the worktime parent command.
//
// These tests are separate from worktime_test.go (legacy TSV path) per the
// no-monoliths rule: each file has one focused responsibility.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: f.legacy.dayoffs, State: f.legacy.active}

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
		SessionCount: func(userID string) (int, error) {
			rows, err := f.sessions.Load(userID)
			if err != nil {
				return 0, err
			}
			return len(rows), nil
		},
		// TSVPath and CacheDBPath left empty so the guard is disabled in most
		// tests; guard-specific tests set them explicitly.
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
	if !strings.Contains(stderr, "Flow") {
		t.Errorf("stderr should mention project name, got %q", stderr)
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
// when nothing is active is a no-op (exit 0, empty stderr).
func TestNewPath_Stop_NothingRunning(t *testing.T) {
	f := newNewPathFixture()
	_, stderr, err := f.run("stop")
	if err != nil {
		t.Fatalf("idle stop must succeed: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty for idle stop, got %q", stderr)
	}
}

// ---- TSV guard tests ----

// guardFixture returns a WorktimeDeps wired with a SessionCount closure and
// configurable TSVPath / CacheDBPath so guard behaviour can be isolated.
type guardFixture struct {
	base        *newPathFixture
	tsvPath     string
	cacheDBPath string
}

func newGuardFixture(t *testing.T) *guardFixture {
	t.Helper()
	dir := t.TempDir()
	return &guardFixture{
		base:        newNewPathFixture(),
		tsvPath:     filepath.Join(dir, "worktime.log"),
		cacheDBPath: filepath.Join(dir, "cache.db"),
	}
}

func (g *guardFixture) deps() cli.WorktimeDeps {
	d := g.base.deps()
	d.TSVPath = g.tsvPath
	d.CacheDBPath = g.cacheDBPath
	return d
}

func (g *guardFixture) run(args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(g.deps())
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// TestGuard_NoTSV_Passthrough verifies that when the legacy worktime.log does
// not exist the guard does nothing and the command proceeds normally.
func TestGuard_NoTSV_Passthrough(t *testing.T) {
	g := newGuardFixture(t)
	// tsvPath does not exist (created by t.TempDir but file itself not created).
	_, stderr, err := g.run("start", "--project=flow")
	// The command itself may fail (no such project / Allgemein auto-create
	// succeeds) — what matters is it does NOT fail with the guard error.
	if err != nil && strings.Contains(err.Error(), "migrate-from-tsv") {
		t.Errorf("guard should not trigger when TSV absent; got: %v", err)
	}
	_ = stderr
}

// TestGuard_TSVExists_EmptyCache_Blocks verifies that when the TSV file
// exists AND the sqlite cache has 0 sessions, the guard returns an error
// mentioning migrate-from-tsv.
func TestGuard_TSVExists_EmptyCache_Blocks(t *testing.T) {
	g := newGuardFixture(t)
	// Create the TSV file.
	if err := os.WriteFile(g.tsvPath, []byte("2026-01-01\t09:00\t17:00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Cache is empty (no sessions seeded).
	_, _, err := g.run("start")
	if err == nil {
		t.Fatal("expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "migrate-from-tsv") {
		t.Errorf("error should mention migrate-from-tsv, got: %v", err)
	}
	if !strings.Contains(err.Error(), g.tsvPath) {
		t.Errorf("error should contain TSV path %q, got: %v", g.tsvPath, err)
	}
}

// TestGuard_TSVExists_CachePopulated_Passthrough verifies that when the TSV
// file exists AND the sqlite cache already has sessions the guard allows the
// command through (user has already migrated).
func TestGuard_TSVExists_CachePopulated_Passthrough(t *testing.T) {
	g := newGuardFixture(t)
	// Create TSV.
	if err := os.WriteFile(g.tsvPath, []byte("2026-01-01\t09:00\t17:00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Seed one session so SessionCount returns 1.
	_ = g.base.sessions.Upsert(domain.Session{
		ID:    "guard-seed",
		Date:  time.Now(),
		Start: time.Now().Add(-time.Hour),
		Stop:  time.Now(),
	})

	_, _, err := g.run("start")
	if err != nil && strings.Contains(err.Error(), "migrate-from-tsv") {
		t.Errorf("guard must not trigger when cache is populated; got: %v", err)
	}
}

// TestGuard_AppliesTo_StopAsWell verifies the guard fires on `stop` not just
// `start` (PersistentPreRunE covers all subcommands).
func TestGuard_AppliesTo_StopAsWell(t *testing.T) {
	g := newGuardFixture(t)
	if err := os.WriteFile(g.tsvPath, []byte("2026-01-01\t09:00\t17:00\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := g.run("stop")
	if err == nil {
		t.Fatal("expected guard error on stop, got nil")
	}
	if !strings.Contains(err.Error(), "migrate-from-tsv") {
		t.Errorf("guard should fire on stop too, got: %v", err)
	}
}
