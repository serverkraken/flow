package worktime_test

// conflicts_test.go — tests for the sync-conflict channel listener and overlay
// wiring in the worktime root (Task 31).
//
// Test matrix:
//   1. conflictReceivedMsg with Resource="sessions" → sessions overlay set on Model;
//      View() contains the conflict overlay text.
//   2. conflictReceivedMsg with Resource="active_sessions" → active-race overlay
//      rendered.
//   3. Listener Cmd: nil Conflicts dep → Init returns no listener; nil chan → nil cmd.
//   4. Resolve flow (sessions): KeyPress 's' → overlay closed, conflictOverlay gone.
//   5. ActiveSession takeover: KeyPress 't' on active-race overlay → invokes
//      ActiveSessions.ForceTakeover(userID, projectID, serverVersion).
//   6. Cancel: KeyPress 'esc' on overlay → overlay closed, no state change.
//   7. Cast failure: ConflictMsg with Resource="sessions" but Local/Server are wrong
//      types → generic-fallback overlay rendered (body contains "Details fehlen").

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// conflictRig extends newRig with sync-conflict deps.
type conflictRig struct {
	rig
	activeStore    *testutil.FakeActiveSessionStoreV2
	queue          *testutil.FakeWriteQueue
	projectStore   *testutil.FakeProjectStore
	activeSessions *usecase.ActiveSessions
	conflictCh     chan ports.ConflictMsg
}

// newConflictRig builds a worktime.Model with Conflicts and ActiveSessions
// wired. The caller can close or send to conflictCh.
func newConflictRig(t *testing.T) conflictRig {
	t.Helper()
	const userID = "u1"

	r := newRig(t)

	projectStore := &testutil.FakeProjectStore{}
	activeStore := &testutil.FakeActiveSessionStoreV2{}
	queue := &testutil.FakeWriteQueue{}

	activeSessions := usecase.NewActiveSessions(nil, projectStore, activeStore, r.sessions, queue)

	conflictCh := make(chan ports.ConflictMsg, 4)

	deps := worktimeDepsFromRig(r)
	deps.ActiveSessions = activeSessions
	deps.UserID = userID
	deps.Conflicts = conflictCh
	// Sync is intentionally nil (M2 mode) for most tests; individual tests that
	// need AcceptServerVersion/OverwriteServerVersion will assert the stub path.

	return conflictRig{
		rig:            rig{model: worktime.New(theme.Load(), deps), clock: r.clock, sessions: r.sessions, active: r.active, dayoffs: r.dayoffs, lock: r.lock, links: r.links, noteLauncher: r.noteLauncher, noteReader: r.noteReader},
		activeStore:    activeStore,
		queue:          queue,
		projectStore:   projectStore,
		activeSessions: activeSessions,
		conflictCh:     conflictCh,
	}
}

// worktimeDepsFromRig extracts the worktime.Deps from an existing rig,
// mirroring the dependency assembly in model_test.go's newRig.
func worktimeDepsFromRig(r rig) worktime.Deps {
	cfg := &testutil.FakeConfigReader{}
	targets := &usecase.TargetResolver{Config: cfg, DayOffs: r.dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: r.sessions, State: r.active, Targets: targets, Clock: r.clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: r.dayoffs, State: r.active}
	return worktime.Deps{
		Reader:        reader,
		Stats:         stats,
		SessionWriter: &usecase.SessionWriter{Sessions: r.sessions, State: r.active, Lock: r.lock, Reader: reader, Clock: r.clock},
		Tagger:        &usecase.Tagger{Sessions: r.sessions},
		DayOffStore:   r.dayoffs,
		DayOffWriter:  &usecase.DayOffWriter{Store: r.dayoffs},
		LinkReader:    &usecase.LinkReader{Store: r.links},
		LinkWriter:    &usecase.LinkWriter{Store: r.links},
		Reporter:      &usecase.Reporter{Reader: reader, DayOffs: r.dayoffs, Targets: targets, Stats: stats, Clock: r.clock},
		NoteOpener:    &usecase.NoteOpener{Launcher: r.noteLauncher},
		NoteReader:    r.noteReader,
		Clock:         r.clock,
	}
}

// sendConflictAndUpdate sends a ConflictMsg to the channel and waits for the
// worktime model to process it. Returns the updated model.
func sendConflictAndUpdate(t *testing.T, m tea.Model, cr conflictRig, msg ports.ConflictMsg) tea.Model {
	t.Helper()
	// Put the message in the channel. The listener goroutine unblocks and
	// returns conflictReceivedMsg — but because we started the listener in
	// Init's drainCmd, the goroutine is already scheduled. We can't re-drain
	// drainCmd here because the listener is a separate goroutine that was
	// already invoked. Instead, we drive it manually: construct the update
	// directly with the exported conflictReceivedMsg equivalent by sending to
	// the channel and having a short-deadline consumer.
	//
	// Practical approach: channels are buffered (cap 4). We put the message in
	// and then call drainCmd with a fresh Cmd (a one-shot read from the same
	// channel pointer held in the rig).
	//
	// Since listenForConflicts is internal, we test via behaviour: send on
	// channel → drainCmd(Init) picks it up → model reflects the change.
	// For the "after-Init" case we simply reload.
	cr.conflictCh <- msg
	// Re-run Init to get a fresh listener and drain it.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	loaded := drainCmd(t, updated, updated.Init())
	return loaded
}

// ── Test 1: sessions conflict → sessions overlay ──────────────────────────

func TestConflict_SessionsMsg_SetsSessionsOverlay(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute), Tag: "deep"}
	server := domain.Session{Start: t0, Stop: t0.Add(105 * time.Minute), Tag: "deep", Note: "touched on phone"}

	msg := ports.ConflictMsg{
		Resource: "sessions",
		RowID:    "sess-1",
		QueueSeq: 42,
		Local:    local,
		Server:   server,
	}

	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	// FilterActive must be true while the overlay is up.
	if !m.(worktime.Model).FilterActive() {
		t.Error("FilterActive should be true after conflictReceivedMsg (sessions)")
	}
	// View must contain the sessions-conflict overlay text (ANSI-stripped).
	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Sync-Konflikt") {
		t.Errorf("expected 'Sync-Konflikt' in overlay view; got:\n%s", out)
	}
	// Both [s] and [l] hints must appear.
	for _, hint := range []string{"[s]", "[l]"} {
		if !strings.Contains(out, hint) {
			t.Errorf("sessions overlay should contain %q; got:\n%s", hint, out)
		}
	}
	// Tab strip must NOT appear — conflict overlay is full-screen.
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if strings.Contains(out, label) {
			t.Errorf("conflict overlay must bypass titlebox; found tab label %q; got:\n%s", label, out)
		}
	}
}

// ── Test 2: active_sessions conflict → active-race overlay ────────────────

func TestConflict_ActiveSessionsMsg_SetsActiveRaceOverlay(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	server := domain.ActiveSession{
		UserID:          "u1",
		ProjectID:       "flow",
		StartedAt:       time.Now().Add(-7 * time.Minute),
		StartedOnDevice: "notebook-b",
		Version:         5,
	}

	msg := ports.ConflictMsg{
		Resource: "active_sessions",
		RowID:    "flow",
		QueueSeq: 7,
		Server:   server,
	}

	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	if !m.(worktime.Model).FilterActive() {
		t.Error("FilterActive should be true after active_sessions conflictReceivedMsg")
	}
	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Aktive Session") {
		t.Errorf("expected 'Aktive Session' in overlay; got:\n%s", out)
	}
	// [t] and [n] hints for takeover and parallel.
	for _, hint := range []string{"[t]", "[n]"} {
		if !strings.Contains(out, hint) {
			t.Errorf("active-race overlay should contain %q; got:\n%s", hint, out)
		}
	}
}

// ── Test 3: nil Conflicts dep → no listener spawned ───────────────────────

func TestConflict_NilConflictsDep_NoListenerInInit(t *testing.T) {
	t.Parallel()
	// Build a model WITHOUT Deps.Conflicts.
	r := newRig(t)
	m := r.model
	// Init should still return a Cmd (at least the tick), but the conflict
	// listener specifically must NOT block forever on a nil channel.
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return a Cmd (tick scheduler at minimum)")
	}
	// Verify that drainCmd completes without hanging — a nil-channel listener
	// would block forever and trigger the 100ms deadline in drainCmd.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	result := drainCmd(t, updated, updated.Init())
	// After draining, FilterActive should still be false (no overlay).
	if result.(worktime.Model).FilterActive() {
		t.Error("FilterActive should be false when no conflict was received")
	}
}

// ── Test 4: resolve sessions [s] → overlay closed, Sync nil → stub toast ──

func TestConflict_SessionsResolveServer_ClosesOverlay(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute)}
	server := domain.Session{Start: t0, Stop: t0.Add(100 * time.Minute)}

	msg := ports.ConflictMsg{Resource: "sessions", RowID: "s1", QueueSeq: 1, Local: local, Server: server}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	// Verify overlay is up.
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("setup: overlay should be active before key press")
	}

	// Press [s] → conflictResolveServerMsg → overlay closed.
	// Deps.Sync is nil (M2 stub path).
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	final := drainCmd(t, updated, cmd)

	if final.(worktime.Model).FilterActive() {
		t.Error("overlay should be closed after pressing [s] to accept server version")
	}
	out := ansi.Strip(final.View().Content)
	// Overlay is gone — tab strip should be visible again.
	if !strings.Contains(out, "Heute") {
		t.Errorf("after overlay close, worktime body should be visible; got:\n%s", out)
	}
}

func TestConflict_SessionsResolveLocal_ClosesOverlay(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute)}
	server := domain.Session{Start: t0, Stop: t0.Add(100 * time.Minute)}

	msg := ports.ConflictMsg{Resource: "sessions", RowID: "s1", QueueSeq: 2, Local: local, Server: server}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "l"})
	final := drainCmd(t, updated, cmd)

	if final.(worktime.Model).FilterActive() {
		t.Error("overlay should be closed after pressing [l] to overwrite with local")
	}
}

// ── Test 5: active-race [t] → ForceTakeover called ────────────────────────

func TestConflict_ActiveRaceTakeover_CallsForceTakeover(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	const projectID = "flow"
	const serverVersion int64 = 5

	server := domain.ActiveSession{
		UserID:    "u1",
		ProjectID: projectID,
		StartedAt: time.Now().Add(-5 * time.Minute),
		Version:   serverVersion,
	}

	msg := ports.ConflictMsg{Resource: "active_sessions", RowID: projectID, QueueSeq: 3, Server: server}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	if !m.(worktime.Model).FilterActive() {
		t.Fatal("setup: active-race overlay should be up")
	}

	// Press [t] → conflictTakeoverMsg.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "t"})
	final := drainCmd(t, updated, cmd)

	// Overlay must be gone.
	if final.(worktime.Model).FilterActive() {
		t.Error("overlay should be closed after [t] takeover")
	}
	// ForceTakeover should have upserted a new active session.
	as, err := cr.activeStore.Get("u1", projectID)
	if err != nil {
		t.Fatalf("expected active session after ForceTakeover, got: %v", err)
	}
	if as.ProjectID != projectID {
		t.Errorf("active session projectID = %q, want %q", as.ProjectID, projectID)
	}
	// The write queue should have received a new entry with the server version
	// as expectedVersion (the If-Match semantics).
	if len(cr.queue.Entries) == 0 {
		t.Error("ForceTakeover should have enqueued a payload")
	}
	lastEntry := cr.queue.Entries[len(cr.queue.Entries)-1]
	if lastEntry.ExpectedVersion != serverVersion {
		t.Errorf("queue entry ExpectedVersion = %d, want %d", lastEntry.ExpectedVersion, serverVersion)
	}
}

// ── Test 6: [esc] → overlay closed, no state change ──────────────────────

func TestConflict_Esc_ClosesOverlayWithoutAction(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute)}
	server := domain.Session{Start: t0, Stop: t0.Add(100 * time.Minute)}

	msg := ports.ConflictMsg{Resource: "sessions", RowID: "s1", QueueSeq: 99, Local: local, Server: server}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	if !m.(worktime.Model).FilterActive() {
		t.Fatal("setup: overlay should be active")
	}

	// Press Esc.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	final := drainCmd(t, updated, cmd)

	if final.(worktime.Model).FilterActive() {
		t.Error("overlay should be closed after Esc")
	}
	// No active session should have been created.
	if len(cr.queue.Entries) != 0 {
		t.Errorf("Esc must not enqueue any write; got %d entries", len(cr.queue.Entries))
	}
}

// ── Test 7: cast failure → generic-fallback overlay ───────────────────────

func TestConflict_CastFailure_RendersGenericFallback(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	// Deliberately wrong payload types for a "sessions" conflict.
	msg := ports.ConflictMsg{
		Resource: "sessions",
		RowID:    "bad",
		QueueSeq: 50,
		Local:    "unexpected-string-type",
		Server:   42, // wrong type
	}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	if !m.(worktime.Model).FilterActive() {
		t.Error("FilterActive should be true even for cast-failure (generic overlay shown)")
	}
	out := ansi.Strip(m.View().Content)
	// Generic fallback must mention "Details fehlen".
	if !strings.Contains(out, "Details fehlen") {
		t.Errorf("generic fallback overlay should contain 'Details fehlen'; got:\n%s", out)
	}
}

// ── Test 8: active-race [n] → parallel → overlay closed, no takeover ─────

func TestConflict_ActiveRaceParallel_ClosesOverlay(t *testing.T) {
	t.Parallel()
	cr := newConflictRig(t)

	server := domain.ActiveSession{
		UserID:    "u1",
		ProjectID: "flow",
		StartedAt: time.Now().Add(-3 * time.Minute),
		Version:   2,
	}
	msg := ports.ConflictMsg{Resource: "active_sessions", RowID: "flow", QueueSeq: 4, Server: server}
	m := sendConflictAndUpdate(t, cr.model, cr, msg)

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "n"})
	final := drainCmd(t, updated, cmd)

	if final.(worktime.Model).FilterActive() {
		t.Error("overlay should be closed after [n] (parallel)")
	}
	// No write queue entry — parallel just closes the overlay.
	if len(cr.queue.Entries) != 0 {
		t.Errorf("[n] must not enqueue any write; got %d entries", len(cr.queue.Entries))
	}
}
