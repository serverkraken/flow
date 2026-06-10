package worktime

// White-box tests for the P2 picker-switch behaviour. These live in
// package worktime (not worktime_test) so they can inject the unexported
// pickerPickedMsg directly, which is cleaner than adding a public test-hook
// just for message injection.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// switchPickerRig is a minimal rig that wires ActiveSessions + Projects +
// UserID (the new-path deps). Only the fields needed for the switch tests
// are exposed; the legacy SessionWriter and DayOff wiring follows the same
// pattern as newPickerRig in today_project_picker_test.go.
type switchPickerRig struct {
	model        heute
	clock        *testutil.FixedClock
	sessions     *testutil.FakeSessionStore
	projectStore *testutil.FakeProjectStore
	activeStore  *testutil.FakeActiveSessionStoreV2
	queue        *testutil.FakeWriteQueue
}

func newSwitchPickerRig(t *testing.T) switchPickerRig {
	t.Helper()
	const userID = "user-switch-test"

	clock := &testutil.FixedClock{T: time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{}
	legacyActive := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	links := &testutil.FakeLinkStore{}
	lock := &testutil.FakeLock{}

	projectStore := &testutil.FakeProjectStore{}
	activeStore := &testutil.FakeActiveSessionStoreV2{}
	queue := &testutil.FakeWriteQueue{}

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: legacyActive, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: dayoffs}

	activeSessions := usecase.NewActiveSessions(nil, projectStore, activeStore, sessions, queue)
	projects := usecase.NewProjects(nil, projectStore, nil)

	pal := theme.Load()
	deps := Deps{
		Reader:         reader,
		Stats:          stats,
		SessionWriter:  &usecase.SessionWriter{Sessions: sessions, State: legacyActive, Lock: lock, Reader: reader, Clock: clock},
		Tagger:         &usecase.Tagger{Sessions: sessions},
		DayOffStore:    dayoffs,
		DayOffWriter:   &usecase.DayOffWriter{Store: dayoffs},
		LinkReader:     &usecase.LinkReader{Store: links},
		LinkWriter:     &usecase.LinkWriter{Store: links},
		Reporter:       &usecase.Reporter{Reader: reader, DayOffs: dayoffs, Targets: targets, Stats: stats, Clock: clock},
		Clock:          clock,
		Projects:       projects,
		ActiveSessions: activeSessions,
		UserID:         userID,
	}

	m := newHeute(pal, deps)

	return switchPickerRig{
		model:        m,
		clock:        clock,
		sessions:     sessions,
		projectStore: projectStore,
		activeStore:  activeStore,
		queue:        queue,
	}
}

// drainSwitchCmd executes cmd, feeds results back into m, and recurses on
// any returned cmd. Batches are unwrapped. Blocks up to 200 ms per leg;
// tick-like cmds that block longer are silently dropped.
func drainSwitchCmd(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msgCh := make(chan tea.Msg, 1)
	go func() {
		defer func() { _ = recover() }()
		msgCh <- cmd()
	}()
	var msg tea.Msg
	select {
	case msg = <-msgCh:
	case <-time.After(200 * time.Millisecond):
		return m
	}
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainSwitchCmd(t, m, c)
		}
		return m
	}
	updated, next := m.Update(msg)
	return drainSwitchCmd(t, updated, next)
}

// loadedSwitchHeute sizes the model and drains Init so h.activeSessions is
// populated before the test begins.
func loadedSwitchHeute(t *testing.T, pr switchPickerRig) tea.Model {
	t.Helper()
	m := tea.Model(pr.model)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return drainSwitchCmd(t, updated, updated.Init())
}

// TestPickerSwitchProjectSequencesStopThenStart verifies the atomic switch
// path: a session runs on project A; the user picks project B in the picker;
// Stop(A) is called then Start(B) is called in a single Cmd. After draining,
// project A has no active session and project B has one.
func TestPickerSwitchProjectSequencesStopThenStart(t *testing.T) {
	pr := newSwitchPickerRig(t)
	pr.projectStore.Projects = []domain.Project{
		{ID: "proj-A", UserID: "user-switch-test", Name: "Alpha", Slug: "alpha"},
		{ID: "proj-B", UserID: "user-switch-test", Name: "Beta", Slug: "beta"},
	}
	_ = pr.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-switch-test",
		ProjectID: "proj-A",
		StartedAt: pr.clock.T.Add(-30 * time.Minute),
	})

	m := loadedSwitchHeute(t, pr)

	// Open picker.
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})
	wt, ok := m.(heute)
	if !ok {
		t.Fatal("expected heute model type")
	}
	if wt.pp == nil {
		t.Fatal("setup: picker should be open after `s`")
	}

	// Inject pickerPickedMsg for project B directly (avoids fragile cursor
	// assumptions about the rendered picker list).
	updated, cmd := m.Update(pickerPickedMsg{projectID: "proj-B", projectName: "Beta"})
	_ = drainSwitchCmd(t, updated, cmd)

	// proj-A must be gone (Stop was called).
	_, errA := pr.activeStore.Get("user-switch-test", "proj-A")
	if errA == nil {
		t.Error("switch: project A must be stopped (no active session), but one still exists")
	}
	// proj-B must now be running (Start was called).
	asB, errB := pr.activeStore.Get("user-switch-test", "proj-B")
	if errB != nil {
		t.Fatalf("switch: project B must have an active session after switch, got: %v", errB)
	}
	if asB.ProjectID != "proj-B" {
		t.Errorf("switch: active session project = %q, want proj-B", asB.ProjectID)
	}
}

// TestPickerSelfPickIsNoop verifies that picking the currently-running project
// from the picker is a no-op: neither Stop nor Start is called and the session
// remains. The picker closes and the sessions-store is unchanged.
func TestPickerSelfPickIsNoop(t *testing.T) {
	pr := newSwitchPickerRig(t)
	pr.projectStore.Projects = []domain.Project{
		{ID: "proj-A", UserID: "user-switch-test", Name: "Alpha", Slug: "alpha"},
	}
	_ = pr.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-switch-test",
		ProjectID: "proj-A",
		StartedAt: pr.clock.T.Add(-15 * time.Minute),
	})

	m := loadedSwitchHeute(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})
	wt, ok := m.(heute)
	if !ok {
		t.Fatal("expected heute model type")
	}
	if wt.pp == nil {
		t.Fatal("setup: picker should be open after `s`")
	}

	// Record sessions-store size before the pick.
	beforeSessions := len(pr.sessions.Sessions)

	// Pick project A — same as the running one.
	updated, cmd := m.Update(pickerPickedMsg{projectID: "proj-A", projectName: "Alpha"})
	_ = drainSwitchCmd(t, updated, cmd)

	// Picker must be closed.
	if updatedHH, ok2 := updated.(heute); ok2 {
		if updatedHH.pp != nil {
			t.Error("self-pick: picker must close after picking the already-running project")
		}
	}
	// Session store must not have gained a new entry (no Stop→Session written).
	if len(pr.sessions.Sessions) != beforeSessions {
		t.Errorf("self-pick: session store size changed (%d → %d); Stop must not have been called",
			beforeSessions, len(pr.sessions.Sessions))
	}
	// Project A must still be running.
	_, err := pr.activeStore.Get("user-switch-test", "proj-A")
	if err != nil {
		t.Error("self-pick: project A must still have an active session after self-pick no-op")
	}
}
