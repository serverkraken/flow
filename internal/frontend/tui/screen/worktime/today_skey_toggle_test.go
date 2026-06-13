package worktime

// White-box tests for the A3 s-key toggle: stop/resume/idle dispatch in
// server mode (ActiveSessions wired). Lives in package worktime so we can
// reach unexported helpers and inject pickerPickedMsg without an exported
// test hook.
//
// Running/paused predicate: h.day.IsRunning() and h.day.IsPaused() are
// the canonical signals (they drive the footer hints). In the test rig both
// the V2 activeStore (for the ActiveSessions use-case path) AND the legacy
// FakeActiveSessionStore (for the WorktimeReader / Day) are seeded together
// so the two layers agree on the state the TUI displays.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// sKeyRig is a minimal rig wiring ActiveSessions + Projects + UserID
// alongside the legacy deps needed by WorktimeReader / SessionWriter.
// It mirrors newSwitchPickerRig from today_project_picker_switch_test.go.
type sKeyRig struct {
	model        heute
	clock        *testutil.FixedClock
	sessions     *testutil.FakeSessionStore
	legacyActive *testutil.FakeActiveSessionStore // drives h.day.IsRunning()/IsPaused()
	projectStore *testutil.FakeProjectStore
	activeStore  *testutil.FakeActiveSessionStoreV2 // drives h.activeSessions
}

func newSKeyRig(t *testing.T) sKeyRig {
	t.Helper()
	const userID = "user-skey-test"

	clock := &testutil.FixedClock{T: time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{}
	legacyActive := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	links := &testutil.FakeLinkStore{}
	lock := &testutil.FakeLock{}

	projectStore := &testutil.FakeProjectStore{}
	activeStore := &testutil.FakeActiveSessionStoreV2{}

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: legacyActive, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: dayoffs}

	machine := testutil.NewFakeWorktimeMachine(userID, activeStore, sessions)
	activeSessions := usecase.NewActiveSessions(nil, projectStore, activeStore, machine)
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

	return sKeyRig{
		model:        m,
		clock:        clock,
		sessions:     sessions,
		legacyActive: legacyActive,
		projectStore: projectStore,
		activeStore:  activeStore,
	}
}

// loadedSKeyHeute sizes the model and drains Init so h.activeSessions is
// populated before the test begins.
func loadedSKeyHeute(t *testing.T, r sKeyRig) tea.Model {
	t.Helper()
	m := tea.Model(r.model)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return drainSwitchCmd(t, updated, updated.Init())
}

// TestSKeyRunning_StopsActiveSession verifies that pressing `s` while a
// session is running (h.day.IsRunning() && len(h.activeSessions) > 0) calls
// ActiveSessions.Stop. The session must be removed from activeStore and the
// picker must NOT open (h.pp == nil).
func TestSKeyRunning_StopsActiveSession(t *testing.T) {
	r := newSKeyRig(t)
	r.projectStore.Projects = []domain.Project{
		{ID: "proj-run", UserID: "user-skey-test", Name: "Runner", Slug: "runner"},
	}
	// Seed V2 active session so h.activeSessions is populated.
	_ = r.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-skey-test",
		ProjectID: "proj-run",
		StartedAt: r.clock.T.Add(-30 * time.Minute),
	})
	// Seed legacy active marker so h.day.IsRunning() returns true.
	start := r.clock.T.Add(-30 * time.Minute)
	r.legacyActive.Active = &start

	m := loadedSKeyHeute(t, r)

	// Press `s` — should stop the session, NOT open the picker.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainSwitchCmd(t, updated, cmd)

	// Picker must NOT be open.
	if hh, ok := updated.(heute); ok && hh.pp != nil {
		t.Error("s while running: picker must NOT open (toggle stop path)")
	}

	// The active session must be gone (Stop was called).
	_, err := r.activeStore.Get("user-skey-test", "proj-run")
	if err == nil {
		t.Error("s while running: ActiveSessions.Stop must remove the active session row")
	}
}

// TestSKeyPaused_ResumesActiveSession verifies that pressing `s` while a
// session is paused (h.day.IsPaused() && len(h.activeSessions) > 0) calls
// ActiveSessions.Resume. The active session row must remain with PausedAt
// cleared and the picker must NOT open.
func TestSKeyPaused_ResumesActiveSession(t *testing.T) {
	r := newSKeyRig(t)
	r.projectStore.Projects = []domain.Project{
		{ID: "proj-pause", UserID: "user-skey-test", Name: "Pauser", Slug: "pauser"},
	}
	pausedAt := r.clock.T.Add(-15 * time.Minute)
	// Seed V2 active session with PausedAt set so the row exists for Resume.
	_ = r.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-skey-test",
		ProjectID: "proj-pause",
		StartedAt: r.clock.T.Add(-45 * time.Minute),
		PausedAt:  &pausedAt,
	})
	// Seed legacy pause marker so h.day.IsPaused() returns true
	// (Active == nil, PausedAt set).
	r.legacyActive.Active = nil
	r.legacyActive.Pause = &pausedAt

	m := loadedSKeyHeute(t, r)

	// Press `s` — should resume, NOT open the picker.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainSwitchCmd(t, updated, cmd)

	// Picker must NOT be open.
	if hh, ok := updated.(heute); ok && hh.pp != nil {
		t.Error("s while paused: picker must NOT open (toggle resume path)")
	}

	// Active session must still exist (Resume is non-destructive).
	as, err := r.activeStore.Get("user-skey-test", "proj-pause")
	if err != nil {
		t.Fatalf("s while paused: active session must remain after Resume, got: %v", err)
	}
	// PausedAt must be cleared by Resume.
	if as.PausedAt != nil {
		t.Error("s while paused: PausedAt must be nil after Resume")
	}
}

// TestSKeyIdle_OpensProjectPicker verifies that pressing `s` when
// ActiveSessions is wired but no session is running or paused opens the
// project picker (h.pp != nil). No Stop or Resume is called.
func TestSKeyIdle_OpensProjectPicker(t *testing.T) {
	r := newSKeyRig(t)
	r.projectStore.Projects = []domain.Project{
		{ID: "proj-idle", UserID: "user-skey-test", Name: "Idler", Slug: "idler"},
	}
	// No active session seeded — idle state. legacyActive is also zero-value.

	m := loadedSKeyHeute(t, r)

	// Press `s` — should open the picker.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "s"})

	hh, ok := updated.(heute)
	if !ok {
		t.Fatal("expected heute model type after `s`")
	}
	if hh.pp == nil {
		t.Error("s while idle: project picker must open (h.pp != nil)")
	}
	// No active session must have been created (Stop/Resume not called).
	_, err := r.activeStore.Get("user-skey-test", "proj-idle")
	if err == nil {
		t.Error("s while idle: no active session must exist before user picks from picker")
	}
}
