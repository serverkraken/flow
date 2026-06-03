package worktime_test

// Unit tests for the project_picker integration on the `s` key (Task 17).
//
// Test matrix:
//   - Legacy mode (nil deps): `s` falls through to SessionWriter unchanged.
//   - New path: `s` opens project_picker (FilterActive=true, FullScreen=true).
//   - pickerPickedMsg dispatches ActiveSessions.Start.
//   - pickerCreateMsg chains Projects.Create + ActiveSessions.Start.
//   - pickerCancelMsg (Esc) closes the picker.
//   - Active-session indicator renders multi-project running line.
//   - Tab keys gated while picker open (FilterActive).
//   - 'q' treated as text input while picker open (TextInputActive).

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// pickerRig groups all fakes used by the new-path tests.
type pickerRig struct {
	model        worktime.Model
	clock        *testutil.FixedClock
	sessions     *testutil.FakeSessionStore
	active       *testutil.FakeActiveSessionStore
	dayoffs      *testutil.FakeDayOffStore
	lock         *testutil.FakeLock
	links        *testutil.FakeLinkStore
	noteLauncher *testutil.FakeNoteLauncher
	noteReader   *fakeNoteReader

	projectStore   *testutil.FakeProjectStore
	activeStore    *testutil.FakeActiveSessionStoreV2
	queue          *testutil.FakeWriteQueue
	activeSessions *usecase.ActiveSessions
	projects       *usecase.Projects
	userID         string
}

// newPickerRig builds a fully wired worktime root with both the legacy deps
// AND the new ActiveSessions + Projects + UserID deps wired. The `s` key
// will use the project-picker path in this rig.
func newPickerRig(t *testing.T) pickerRig {
	t.Helper()
	const userID = "user-test"

	clock := &testutil.FixedClock{T: time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{}
	legacyActive := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	links := &testutil.FakeLinkStore{}
	noteLauncher := &testutil.FakeNoteLauncher{}
	noteReader := &fakeNoteReader{Bodies: map[string]string{}}
	lock := &testutil.FakeLock{}

	projectStore := &testutil.FakeProjectStore{}
	activeStore := &testutil.FakeActiveSessionStoreV2{}
	queue := &testutil.FakeWriteQueue{}

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: legacyActive, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: dayoffs, State: legacyActive}

	activeSessions := usecase.NewActiveSessions(nil, projectStore, activeStore, sessions, queue)
	projects := usecase.NewProjects(nil, projectStore)

	deps := worktime.Deps{
		Reader:         reader,
		Stats:          stats,
		SessionWriter:  &usecase.SessionWriter{Sessions: sessions, State: legacyActive, Lock: lock, Reader: reader, Clock: clock},
		Tagger:         &usecase.Tagger{Sessions: sessions},
		DayOffStore:    dayoffs,
		DayOffWriter:   &usecase.DayOffWriter{Store: dayoffs},
		LinkReader:     &usecase.LinkReader{Store: links},
		LinkWriter:     &usecase.LinkWriter{Store: links},
		Reporter:       &usecase.Reporter{Reader: reader, DayOffs: dayoffs, Targets: targets, Stats: stats, Clock: clock},
		NoteOpener:     &usecase.NoteOpener{Launcher: noteLauncher},
		NoteReader:     noteReader,
		Clock:          clock,
		Projects:       projects,
		ActiveSessions: activeSessions,
		UserID:         userID,
	}

	return pickerRig{
		model:          worktime.New(theme.Load(), deps),
		clock:          clock,
		sessions:       sessions,
		active:         legacyActive,
		dayoffs:        dayoffs,
		lock:           lock,
		links:          links,
		noteLauncher:   noteLauncher,
		noteReader:     noteReader,
		projectStore:   projectStore,
		activeStore:    activeStore,
		queue:          queue,
		activeSessions: activeSessions,
		projects:       projects,
		userID:         userID,
	}
}

// loadedHeuteForPicker sizes the model and drains Init so the day + active-
// sessions list are loaded before each test begins.
func loadedHeuteForPicker(t *testing.T, pr pickerRig) tea.Model {
	t.Helper()
	updated, _ := pr.model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return drainCmd(t, updated, updated.Init())
}

// — Legacy mode (unchanged) —

// TestPickerLegacy_SKeyStartsSessionWriter verifies that when ActiveSessions
// and UserID are NOT wired the `s` key still uses the legacy SessionWriter.
func TestPickerLegacy_SKeyStartsSessionWriter(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainCmd(t, updated, cmd)
	if r.active.Active == nil {
		t.Fatal("legacy path: expected active marker after `s` from idle")
	}
}

// TestPickerLegacy_FilterActiveIsFalseWhenNoDialog verifies legacy mode
// reports FilterActive=false when no dialog is open.
func TestPickerLegacy_FilterActiveIsFalseWhenNoDialog(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	if m.(worktime.Model).FilterActive() {
		t.Error("FilterActive should be false in idle legacy mode")
	}
}

// — New path: picker opens —

// TestPicker_SKeyOpensPicker verifies that pressing `s` when new deps are
// wired opens the project picker (FilterActive=true, FullScreen implied by
// no titlebox tab-strip in View).
func TestPicker_SKeyOpensPicker(t *testing.T) {
	pr := newPickerRig(t)
	m := loadedHeuteForPicker(t, pr)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "s"})
	wt := updated.(worktime.Model)
	if !wt.FilterActive() {
		t.Error("s with new path: FilterActive should be true (picker open)")
	}
	out := ansi.Strip(updated.View().Content)
	// Picker title must be visible.
	if !strings.Contains(out, "Projekt wählen") {
		t.Errorf("picker overlay should contain 'Projekt wählen', got:\n%s", out)
	}
	// Tab-strip must NOT appear in the picker view (FullScreen bypasses titlebox).
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if strings.Contains(out, label) {
			t.Errorf("picker overlay must bypass titlebox; found tab label %q in view:\n%s", label, out)
		}
	}
}

// TestPicker_ViewShowsProjectList verifies that projects in the store appear
// in the picker view.
func TestPicker_ViewShowsProjectList(t *testing.T) {
	pr := newPickerRig(t)
	pr.projectStore.Projects = []domain.Project{
		{ID: "1", UserID: "user-test", Name: "flow", Slug: "flow"},
		{ID: "2", UserID: "user-test", Name: "Allgemein", Slug: "allgemein"},
	}
	m := loadedHeuteForPicker(t, pr)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "s"})
	out := ansi.Strip(updated.View().Content)
	for _, want := range []string{"flow", "Allgemein", "+ Neues Projekt anlegen"} {
		if !strings.Contains(out, want) {
			t.Errorf("picker should show %q, got:\n%s", want, out)
		}
	}
}

// TestPicker_EscClosesPickerRestoresFocus verifies that pressing Esc closes
// the picker (FilterActive=false). The Esc cmd returns pickerCancelMsg which
// must be fed back into the model (drainCmd handles this).
func TestPicker_EscClosesPickerRestoresFocus(t *testing.T) {
	pr := newPickerRig(t)
	m := loadedHeuteForPicker(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("setup: picker should be open after `s`")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = drainCmd(t, updated, cmd) // drive the pickerCancelMsg cmd
	if m.(worktime.Model).FilterActive() {
		t.Error("Esc should close picker (FilterActive=false)")
	}
	// After close, the normal body should be visible (no "Projekt wählen").
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "Projekt wählen") {
		t.Errorf("after Esc, picker chrome should not be visible; got:\n%s", out)
	}
}

// TestPicker_PickedProjectStartsSession verifies that selecting a project
// from the picker calls ActiveSessions.Start and writes an active session row.
func TestPicker_PickedProjectStartsSession(t *testing.T) {
	pr := newPickerRig(t)
	pr.projectStore.Projects = []domain.Project{
		{ID: "proj-1", UserID: "user-test", Name: "flow", Slug: "flow"},
	}
	m := loadedHeuteForPicker(t, pr)
	// Open picker.
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})
	// Press Enter — the first real project ("flow") is cursor=0.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)

	as, err := pr.activeStore.Get("user-test", "proj-1")
	if err != nil {
		t.Fatalf("expected active session for proj-1, got error: %v", err)
	}
	if as.ProjectID != "proj-1" {
		t.Errorf("active session project = %q, want proj-1", as.ProjectID)
	}
	// Queue should have received the start payload.
	if len(pr.queue.Entries) == 0 {
		t.Error("expected write queue entry after picker-driven start")
	}
}

// TestPicker_CreateProjectStartsSession verifies that entering a new project
// name via the "+Neu" row calls Projects.Create then ActiveSessions.Start.
func TestPicker_CreateProjectStartsSession(t *testing.T) {
	pr := newPickerRig(t)
	// No pre-existing projects — picker has only "+Neu" row.
	m := loadedHeuteForPicker(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})

	// Type a name that avoids j/k/↑/↓ (those are nav keys in the picker filter).
	// "flowtest" uses only printable chars that are not bound to picker nav.
	for _, ch := range "flowtest" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)

	if len(pr.projectStore.Projects) == 0 {
		t.Fatal("Projects.Create should have added a project to the store")
	}
	created := pr.projectStore.Projects[0]
	if created.Name != "flowtest" {
		t.Errorf("created project name = %q, want 'flowtest'", created.Name)
	}
	as, err := pr.activeStore.Get("user-test", created.ID)
	if err != nil {
		t.Fatalf("expected active session for new project, got: %v", err)
	}
	if as.ProjectID != created.ID {
		t.Errorf("active session project = %q, want %q", as.ProjectID, created.ID)
	}
}

// TestPicker_RunningProjectExcludedFromList verifies that projects with an
// existing active session are excluded from the picker item list so the user
// cannot start a duplicate session for the same project.
func TestPicker_RunningProjectExcludedFromList(t *testing.T) {
	pr := newPickerRig(t)
	pr.projectStore.Projects = []domain.Project{
		{ID: "proj-running", UserID: "user-test", Name: "running-project", Slug: "running-project"},
		{ID: "proj-idle", UserID: "user-test", Name: "idle-project", Slug: "idle-project"},
	}
	// Seed an active session for proj-running so it shows in h.activeSessions.
	_ = pr.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-test",
		ProjectID: "proj-running",
		StartedAt: pr.clock.T.Add(-10 * time.Minute),
	})

	m := loadedHeuteForPicker(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"})
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "running-project") {
		t.Errorf("already-running project should be excluded from picker; got:\n%s", out)
	}
	if !strings.Contains(out, "idle-project") {
		t.Errorf("idle project should appear in picker; got:\n%s", out)
	}
}

// TestPicker_ActiveSessionIndicator_RendersMultiProject verifies that the
// new multi-project running-indicator line appears when active sessions exist.
func TestPicker_ActiveSessionIndicator_RendersMultiProject(t *testing.T) {
	pr := newPickerRig(t)
	_ = pr.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-test",
		ProjectID: "proj-a",
		StartedAt: pr.clock.T.Add(-2*time.Hour - 30*time.Minute),
	})
	_ = pr.activeStore.Upsert(domain.ActiveSession{
		UserID:    "user-test",
		ProjectID: "proj-b",
		StartedAt: pr.clock.T.Add(-12 * time.Minute),
	})

	m := loadedHeuteForPicker(t, pr)
	out := ansi.Strip(m.View().Content)

	// Both elapsed durations must appear.
	for _, want := range []string{"2h 30m", "0h 12m"} {
		if !strings.Contains(out, want) {
			t.Errorf("active session indicator should contain %q; got:\n%s", want, out)
		}
	}
	// The ▶ glyph must appear (at least once per running session).
	if !strings.Contains(out, "▶") {
		t.Errorf("active session indicator must contain ▶ glyph; got:\n%s", out)
	}
}

// TestPicker_QKeyWhilePickerOpen_TypesIntoFilter verifies that pressing 'q'
// while the picker is open types into the filter (the picker stays open) rather
// than quitting the app. We test this indirectly: the picker FilterActive must
// remain true after the key, proving the picker absorbed it.
func TestPicker_QKeyWhilePickerOpen_TypesIntoFilter(t *testing.T) {
	pr := newPickerRig(t)
	m := loadedHeuteForPicker(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"}) // open picker
	// 'q' must go to the picker filter, not bubble up to the worktime quit handler.
	m, _ = m.Update(tea.KeyPressMsg{Text: "q"})
	if !m.(worktime.Model).FilterActive() {
		t.Error("FilterActive must remain true after 'q' while picker is open; picker must not have closed")
	}
}

// TestPicker_Tab1WhilePickerOpen_DoesNotSwitchTabs verifies that pressing
// '1' while the picker is open does NOT switch to a different worktime tab —
// FilterActive gates the tab-router in the worktime root.
func TestPicker_Tab1WhilePickerOpen_DoesNotSwitchTabs(t *testing.T) {
	pr := newPickerRig(t)
	m := loadedHeuteForPicker(t, pr)
	m, _ = m.Update(tea.KeyPressMsg{Text: "s"}) // open picker

	// '1' must type into the filter, not switch to Heute tab.
	m, _ = m.Update(tea.KeyPressMsg{Text: "1"})
	if !m.(worktime.Model).FilterActive() {
		t.Error("FilterActive should remain true while picker is open; '1' must not switch tabs")
	}
}
