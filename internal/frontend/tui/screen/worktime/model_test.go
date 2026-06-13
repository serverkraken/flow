package worktime_test

import (
	"fmt"
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

// fakeNoteReader is a minimal worktime.NoteReader for tests. Bodies
// keyed by note ID; missing IDs surface as a "not found" error so the
// integrated note-view dialog's error path can be exercised.
type fakeNoteReader struct {
	Bodies map[string]string
}

func (f *fakeNoteReader) Read(id string) (string, error) {
	if body, ok := f.Bodies[id]; ok {
		return body, nil
	}
	return "", fmt.Errorf("note %q not found", id)
}

// rig groups the fakes alongside the constructed Model so individual
// tests can both inspect the inputs and manipulate the wired state
// (e.g. seeding sessions before Init runs).
type rig struct {
	model        worktime.Model
	clock        *testutil.FixedClock
	sessions     *testutil.FakeSessionStore
	active       *testutil.FakeActiveSessionStore
	dayoffs      *testutil.FakeDayOffStore
	lock         *testutil.FakeLock
	links        *testutil.FakeLinkStore
	noteLauncher *testutil.FakeNoteLauncher
	noteReader   *fakeNoteReader
}

// newRig builds a wired worktime root with empty fakes everywhere — the
// wiring shape mirrors what the composition root in cmd/flow/main.go
// hands to the live process.
func newRig(t *testing.T) rig {
	t.Helper()
	clock := &testutil.FixedClock{T: time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{}
	active := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	links := &testutil.FakeLinkStore{}
	noteLauncher := &testutil.FakeNoteLauncher{}
	noteReader := &fakeNoteReader{Bodies: map[string]string{}}
	lock := &testutil.FakeLock{}

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: active, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: dayoffs}
	deps := worktime.Deps{
		Reader:        reader,
		Stats:         stats,
		SessionWriter: &usecase.SessionWriter{Sessions: sessions, State: active, Lock: lock, Reader: reader, Clock: clock},
		Tagger:        &usecase.Tagger{Sessions: sessions},
		DayOffStore:   dayoffs,
		DayOffWriter:  &usecase.DayOffWriter{Store: dayoffs},
		LinkReader:    &usecase.LinkReader{Store: links},
		LinkWriter:    &usecase.LinkWriter{Store: links},
		Reporter:      &usecase.Reporter{Reader: reader, DayOffs: dayoffs, Targets: targets, Stats: stats, Clock: clock},
		NoteOpener:    &usecase.NoteOpener{Launcher: noteLauncher},
		NoteReader:    noteReader,
		Clock:         clock,
	}
	return rig{
		model:        worktime.New(theme.Load(), deps),
		clock:        clock,
		sessions:     sessions,
		active:       active,
		dayoffs:      dayoffs,
		lock:         lock,
		links:        links,
		noteLauncher: noteLauncher,
		noteReader:   noteReader,
	}
}

// newModel keeps the original test helper signature. Callers that don't
// need to inspect or seed the fakes can stay one-liners.
func newModel(t *testing.T) worktime.Model {
	t.Helper()
	return newRig(t).model
}

// drainCmd executes a tea.Cmd, recursively unwrapping tea.Batch
// results, and feeds every returned tea.Msg back into the model.
// Returns the final model.
//
// Each cmd is invoked inside a goroutine with a short deadline, so
// tea.Tick commands (which would otherwise block until their timer
// fires) are skipped without stalling the test.
func drainCmd(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
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
	case <-time.After(100 * time.Millisecond):
		// Treat as tea.Tick or other long-blocking cmd — drop it.
		return m
	}
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmd(t, m, c)
		}
		return m
	}
	updated, next := m.Update(msg)
	return drainCmd(t, updated, next)
}

func TestNew_BeforeWindowSize_ViewIsEmpty(t *testing.T) {
	m := newModel(t)
	if got := m.View().Content; got != "" {
		t.Errorf("View before WindowSizeMsg should be empty, got %q", got)
	}
}

// TestView_RendersBodyAndStub guarantees the worktime View still
// surfaces the active sub-model's body (here: the Heute loading
// indicator before Init runs). The sub-tab strip (Heute / Woche /
// History / Frei) was lifted to the sidekick.subTabHost contract in
// Phase 10 of the 2026-05-30 UX-Review-Cleanup — see
// TestSubTabHost_LabelsAndIndex below for the strip exposed via the
// interface. The titlebox title is now the *active* sub-tab name as
// the in-frame "you are here" anchor (Phase-10 follow-up); only the
// current label belongs in the top border, the other three do not.
func TestView_RendersBodyAndStub(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := updated.View().Content
	if !strings.Contains(out, "lädt") {
		t.Errorf("Heute should show its loading indicator before Init runs, got:\n%s", out)
	}
	topLine := out
	if i := strings.Index(out, "\n"); i >= 0 {
		topLine = out[:i]
	}
	// Tab-Strip im title: alle 4 sub-tabs müssen im top border erscheinen,
	// damit der User Heute/Woche/Verlauf/Frei als Navigations-Affordanz
	// erkennt. Aktiver Tab ist Heute (default). ansi.Strip macht den
	// SGR-per-Rune Bold+Underline-Stil des aktiven Tabs für die Substring-
	// Assertion sichtbar.
	topPlain := ansi.Strip(topLine)
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if !strings.Contains(topPlain, label) {
			t.Errorf("worktime top border: expected sub-tab label %q in strip, got %q", label, topPlain)
		}
	}
}

// TestSubTabHost_LabelsAndIndex pins the worktime → sidekick contract:
// SubTabs returns the four sub-tab labels in display order, SubTabIndex
// returns the active sub-tab as a 0-based index. The sidekick consumes
// both to render the right-aligned pill strip.
func TestSubTabHost_LabelsAndIndex(t *testing.T) {
	m := newModel(t)
	got := m.SubTabs()
	want := []string{"Heute", "Woche", "Verlauf", "Frei"}
	if len(got) != len(want) {
		t.Fatalf("SubTabs len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SubTabs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if idx := m.SubTabIndex(); idx != 0 {
		t.Errorf("SubTabIndex default = %d, want 0 (Heute)", idx)
	}
}

// TestSubTabHost_SwitchSubTabUpdatesIndex confirms SwitchSubTab(i)
// returns a new worktime.Model whose SubTabIndex reflects the request,
// and that out-of-range indices are a no-op (defense-in-depth — the
// sidekick already validates but the host invariant stays local).
func TestSubTabHost_SwitchSubTabUpdatesIndex(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	wt := updated.(worktime.Model)
	for i := 0; i < 4; i++ {
		next := wt.SwitchSubTab(i).(worktime.Model)
		if idx := next.SubTabIndex(); idx != i {
			t.Errorf("SwitchSubTab(%d): SubTabIndex = %d, want %d", i, idx, i)
		}
	}
	// Out-of-range: returns the same model with index unchanged.
	guard := wt.SwitchSubTab(-1).(worktime.Model)
	if idx := guard.SubTabIndex(); idx != wt.SubTabIndex() {
		t.Errorf("SwitchSubTab(-1) must be a no-op; index changed from %d to %d", wt.SubTabIndex(), idx)
	}
	guard = wt.SwitchSubTab(4).(worktime.Model)
	if idx := guard.SubTabIndex(); idx != wt.SubTabIndex() {
		t.Errorf("SwitchSubTab(4) must be a no-op; index changed from %d to %d", wt.SubTabIndex(), idx)
	}
}

func TestTabSwitching_NumberKeys(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	cases := []struct {
		key  string
		want string
	}{
		{"2", "Woche lädt"},
		{"3", "Verlauf lädt"},
		{"4", "Frei lädt"},
		// Heute (wave B) renders the live screen — no wave-letter sentinel.
		// Sniff the loading marker, since Init hasn't run for the sub-models
		// in a pure Update-driven test.
		{"1", "Heute lädt"},
	}
	for _, c := range cases {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: c.key})
		if got := updated.View().Content; !strings.Contains(got, c.want) {
			t.Errorf("after key %q expected %q in View, got:\n%s", c.key, c.want, got)
		}
	}
}

func TestTabSwitching_TabCyclesForward(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	wants := []string{"Woche lädt", "Verlauf lädt", "Frei lädt", "Heute lädt"}
	for _, w := range wants {
		updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if got := updated.View().Content; !strings.Contains(got, w) {
			t.Errorf("tab cycle expected %q, got:\n%s", w, got)
		}
	}
}

// TestB_FallsThroughToParentFromAnyTab — `b` claimt Worktime nicht
// mehr (UI-Review): aus jeder Tab soll der Key zur Sidekick-Layer
// durchfallen, die ihn dann als „back to Palette" interpretiert. Vorher
// cyclete `b` rückwärts durch die Tabs, was zwei verschiedene
// Bedeutungen für `b` auf verschiedenen Tabs ergab (Heute → Palette,
// sonst → vorheriger Tab). Doppeldeutig.
func TestB_FallsThroughToParentFromAnyTab(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "4"})
	// 'b' on Frei: Worktime gibt es weiter — sub-model ignoriert es
	// (kein Frei-Keybind), Tab bleibt auf Frei. Wenn der Key konsumiert
	// wäre, würde er entweder zur History cyclen oder Worktime-internes
	// State ändern. Wir asserten daher: View bleibt Frei.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "b"})
	if got := updated.View().Content; !strings.Contains(got, "Frei") {
		t.Errorf("b on Frei must NOT cycle tabs (must fall through to parent); got:\n%s", got)
	}
}

func TestHandlesBack_AlwaysFalse(t *testing.T) {
	// HandlesBack signalisiert dem Sidekick-Parent, dass der Key
	// `b` an Worktime durchgereicht werden soll. Worktime claimt ihn
	// nicht mehr → false auf jedem Tab.
	m := newModel(t)
	if m.HandlesBack() {
		t.Error("HandlesBack must be false on Heute")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "3"})
	if hb, ok := updated.(worktime.Model); !ok || hb.HandlesBack() {
		t.Error("HandlesBack must be false on every tab — `b` is global")
	}
}

func TestInit_ReturnsCmd(t *testing.T) {
	// Only assert Init returns a non-nil tea.Cmd. We don't invoke it —
	// the tick scheduler would block for tickSlow (10 s) in tests.
	if m := newModel(t); m.Init() == nil {
		t.Fatal("Init must return a tea.Cmd (at least the tick scheduler)")
	}
}

func TestStateAccessors_Defaults(t *testing.T) {
	m := newModel(t)
	if m.FilterActive() {
		t.Error("FilterActive should be false on the skeleton stubs")
	}
	// Default tab is heute; StateFilter encodes it as a "tab=…" marker
	// so WithState can restore the tab on the next session.
	if got := m.StateFilter(); got != "tab=heute" {
		t.Errorf("default StateFilter: got %q, want tab=heute", got)
	}
}

// — Heute (wave B) smoke tests —

// loadedHeute returns a worktime root whose Heute sub-model has been
// driven through Init → heuteLoadedMsg, so the day snapshot is populated.
func loadedHeute(t *testing.T, r rig) tea.Model {
	t.Helper()
	updated, _ := r.model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return drainCmd(t, updated, updated.Init())
}

func TestHeute_LoadRendersIdleState(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	out := m.View().Content
	if !strings.Contains(out, "Noch nichts erfasst") {
		t.Errorf("idle Heute should hint at empty state, got:\n%s", out)
	}
	if !strings.Contains(out, "pausiert") {
		t.Errorf("idle Heute should show »pausiert« badge, got:\n%s", out)
	}
}

func TestHeute_LoadRendersRunningState(t *testing.T) {
	r := newRig(t)
	start := r.clock.T.Add(-30 * time.Minute)
	r.active.Active = &start
	m := loadedHeute(t, r)
	out := m.View().Content
	for _, want := range []string{"läuft", "▶", start.Format("15:04")} {
		if !strings.Contains(out, want) {
			t.Errorf("running Heute should contain %q, got:\n%s", want, out)
		}
	}
}

func TestHeute_LoadRendersPausedState(t *testing.T) {
	r := newRig(t)
	pausedAt := r.clock.T.Add(-15 * time.Minute)
	r.active.Pause = &pausedAt
	m := loadedHeute(t, r)
	out := m.View().Content
	if !strings.Contains(out, "in Pause") {
		t.Errorf("paused Heute should surface »in Pause«, got:\n%s", out)
	}
	if !strings.Contains(out, pausedAt.Format("15:04")) {
		t.Errorf("paused Heute should show pause-since time %s, got:\n%s", pausedAt.Format("15:04"), out)
	}
}

func TestHeute_StartKey_StartsSession(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainCmd(t, updated, cmd)
	if r.active.Active == nil {
		t.Fatal("expected active session marker after `s` from idle")
	}
	if got := *r.active.Active; !got.Equal(r.clock.T) {
		t.Errorf("expected start at clock-now %v, got %v", r.clock.T, got)
	}
}

func TestHeute_StopKey_StopsRunningSession(t *testing.T) {
	r := newRig(t)
	start := r.clock.T.Add(-90 * time.Minute)
	r.active.Active = &start
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainCmd(t, updated, cmd)
	if r.active.Active != nil {
		t.Error("expected ClearActive after `s` while running")
	}
	if len(r.sessions.Sessions) != 1 {
		t.Fatalf("expected exactly one logged session, got %d", len(r.sessions.Sessions))
	}
	got := r.sessions.Sessions[0]
	if !got.Start.Equal(start) || !got.Stop.Equal(r.clock.T) {
		t.Errorf("logged span = %s → %s, want %s → %s",
			got.Start.Format("15:04"), got.Stop.Format("15:04"),
			start.Format("15:04"), r.clock.T.Format("15:04"))
	}
}

func TestHeute_PauseKey_RecordsPauseMarker(t *testing.T) {
	r := newRig(t)
	start := r.clock.T.Add(-45 * time.Minute)
	r.active.Active = &start
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p"})
	_ = drainCmd(t, updated, cmd)
	if r.active.Active != nil {
		t.Error("Pause should clear the active marker")
	}
	if r.active.Pause == nil {
		t.Fatal("Pause should set the pause marker")
	}
	if !r.active.Pause.Equal(r.clock.T) {
		t.Errorf("pause marker = %v, want %v", *r.active.Pause, r.clock.T)
	}
}

func TestHeute_ResumeKey_OnPaused(t *testing.T) {
	r := newRig(t)
	pausedAt := r.clock.T.Add(-15 * time.Minute)
	r.active.Pause = &pausedAt
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	_ = drainCmd(t, updated, cmd)
	if r.active.Active == nil {
		t.Fatal("Resume should set Active")
	}
	if r.active.Pause != nil {
		t.Error("Resume should clear the pause marker")
	}
}

// seedSession appends a logged session to the fake store and returns the
// model with Heute fully reloaded so the cursor lands on it.
func seedSessionAndLoad(t *testing.T, r rig, s domain.Session) tea.Model {
	t.Helper()
	r.sessions.Sessions = append(r.sessions.Sessions, s)
	return loadedHeute(t, r)
}

func TestHeute_TagDialog_SetsTag(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)

	// Open tag dialog, type "deep", press Enter.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("tag dialog should set FilterActive=true")
	}
	for _, ch := range "deep" {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	final := drainCmd(t, updated, cmd)
	if fa := final.(worktime.Model); fa.FilterActive() {
		t.Error("dialog should be closed after submit")
	}
	if got := r.sessions.Sessions[0].Tag; got != "deep" {
		t.Errorf("Tag = %q, want %q", got, "deep")
	}
}

func TestHeute_NoteDialog_SetsNote(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "N"})
	for _, ch := range "Standup" {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)
	if got := r.sessions.Sessions[0].Note; got != "Standup" {
		t.Errorf("Note = %q, want %q", got, "Standup")
	}
}

func TestHeute_DeleteDialog_RequiresExplicitConfirm(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)

	// Skill §Keybind grammar: `D` (uppercase) öffnet die destructive Action,
	// confirm.Model bindet y/Enter → ja, n/Esc → nein.
	//
	// Esc cancelt den Dialog ohne Löschen.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(r.sessions.Sessions) != 1 {
		t.Errorf("Esc on delete dialog should cancel, sessions=%d", len(r.sessions.Sessions))
	}

	// Erneut öffnen, mit Enter bestätigen → Session weg.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "D"})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)
	if len(r.sessions.Sessions) != 0 {
		t.Errorf("Enter on delete dialog should confirm and delete, got %d remaining", len(r.sessions.Sessions))
	}
}

func TestHeute_DialogActivatesFilter_GatesTabKeys(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("FilterActive should be true once the tag dialog is open")
	}
	// "2" is the Woche-tab key — but with FilterActive=true it must reach
	// the textinput, not switch tabs.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "2"})
	if strings.Contains(updated.View().Content, "Woche lädt") {
		t.Error("`2` while a Heute dialog is open must not switch to Woche")
	}
}

func TestHeute_EscClosesDialog(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if updated.(worktime.Model).FilterActive() {
		t.Error("Esc should close the tag dialog")
	}
}

// — Heute Wave-B+ slice 1: Kompendium attach (n) + view (o) —

func TestHeute_AttachDialog_AddsNoteToLinkStore(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("attach dialog should set FilterActive=true")
	}
	for _, ch := range "daily-2026-05-01" {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	final := drainCmd(t, updated, cmd)
	if fa := final.(worktime.Model); fa.FilterActive() {
		t.Error("dialog should be closed after submit")
	}
	got := r.links.ByDate[r.clock.T.Format("2006-01-02")]
	if len(got) != 1 || got[0] != "daily-2026-05-01" {
		t.Errorf("LinkStore for today = %v, want [daily-2026-05-01]", got)
	}
}

func TestHeute_AttachDialog_RejectsEmpty(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	// Submit without typing anything → errMsg, dialog stays open, no link added.
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)
	if !updated.(worktime.Model).FilterActive() {
		t.Error("dialog should stay open when submission was rejected")
	}
	if len(r.links.ByDate) != 0 {
		t.Errorf("LinkStore should be empty after empty submit, got %v", r.links.ByDate)
	}
}

func TestHeute_AttachedNotes_RenderAsChipLine(t *testing.T) {
	r := newRig(t)
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	if err := r.links.Add(r.clock.T, "projects/foo-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)
	out := m.View().Content
	// Spec 2026-05-13-filled-dayoff-dots-supersede: attached-notes marker
	// switched from "●" (now Vacation-Identität) to "›" (glyphs.Info) so
	// the chip line doesn't collide with day-off pace dots.
	for _, want := range []string{"›", "daily-2026-05-01", "projects/foo-2026-05-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("attached-notes chip line should contain %q, got:\n%s", want, out)
		}
	}
}

func TestHeute_OpenKey_OpensIntegratedNoteView(t *testing.T) {
	r := newRig(t)
	r.noteReader.Bodies["daily-2026-05-01"] = "# Daily\n\nhello"
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "o"})
	updated = drainCmd(t, updated, cmd)
	// `o` opens the inline note-view dialog (FilterActive=true). The
	// external NoteLauncher.View path was retired in the glow-migration
	// (G2); the integrated renderer is the only path now.
	if !updated.(worktime.Model).FilterActive() {
		t.Error("`o` should activate the inline note-view dialog (FilterActive=true)")
	}
	if len(r.noteLauncher.Calls) != 0 {
		t.Errorf("`o` must not invoke NoteLauncher (external viewer is gone), got Calls=%v", r.noteLauncher.Calls)
	}
}

func TestHeute_OpenKey_NoAttachedNotes_IsNoop(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "o"})
	_ = drainCmd(t, updated, cmd)
	if len(r.noteLauncher.Calls) != 0 {
		t.Errorf("`o` with no attached notes must not launch the viewer, got Calls=%v", r.noteLauncher.Calls)
	}
}

// — Heute Wave-B+ slice 2: O (edit) + Ctrl+D (detach) —

func TestHeute_OpenUppercase_LaunchesEditor(t *testing.T) {
	r := newRig(t)
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "O"})
	_ = drainCmd(t, updated, cmd)
	if len(r.noteLauncher.Calls) != 1 || r.noteLauncher.Calls[0] != "open:daily-2026-05-01" {
		t.Errorf("`O` should call NoteLauncher.Open, got Calls=%v", r.noteLauncher.Calls)
	}
}

func TestHeute_OpenUppercase_NoAttachedNotes_IsNoop(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "O"})
	_ = drainCmd(t, updated, cmd)
	if len(r.noteLauncher.Calls) != 0 {
		t.Errorf("`O` with no attached notes must not launch the editor, got Calls=%v", r.noteLauncher.Calls)
	}
}

func TestHeute_R_DetachesFirstAttachedNote(t *testing.T) {
	r := newRig(t)
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	if err := r.links.Add(r.clock.T, "projects/foo-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)
	// `R` (Remove) statt vorherigem Ctrl+D — Ctrl+D ist die Terminal-EOF-
	// /Process-Kill-Sequenz und las als „destructive" alarmierend.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "R"})
	_ = drainCmd(t, updated, cmd)
	got := r.links.ByDate[r.clock.T.Format("2006-01-02")]
	if len(got) != 1 || got[0] != "projects/foo-2026-05-01" {
		t.Errorf("R should remove the first attached note (daily-...), leaving the second; got %v", got)
	}
}

func TestHeute_R_NoAttachedNotes_IsNoop(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "R"})
	final := drainCmd(t, updated, cmd)
	// Sanity: no panic, no link store mutation, no session deletion either
	// (R must NOT collide with `D`-delete-session).
	if len(r.links.ByDate) != 0 {
		t.Errorf("R with no attachments should not touch the link store, got %v", r.links.ByDate)
	}
	if final.(worktime.Model).FilterActive() {
		t.Error("R should not open any dialog")
	}
}

func TestHeute_HelpOverlay_OpensWithQuestionMark(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("`?` should open the help overlay (FilterActive=true)")
	}
	out := updated.View().Content
	// picker.SectionHeader uppercases its title; sniff the upper form.
	for _, want := range []string{
		"Heute · Hilfe",
		"CURSOR & ACTION",
		"KOMPENDIUM",
		"n", "R", "o", "O",
		"beliebige Taste → schließen",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay should contain %q, got:\n%s", want, out)
		}
	}
}

func TestHeute_HelpOverlay_AnyKeyCloses(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	// Any key dismisses — pick something that would otherwise have its own
	// behaviour in normal mode (`s` toggles start/stop) to prove the help
	// dialog ate the key first.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "s"})
	if updated.(worktime.Model).FilterActive() {
		t.Error("any key on help overlay should close it")
	}
	if r.active.Active != nil {
		t.Error("the dismiss key must not bubble through to start a session")
	}
}

func TestHeute_DDelete_StillDeletesSessionWhenNotesAttached(t *testing.T) {
	// Regression guard: introducing Ctrl+D for detach must not change
	// the meaning of `D` (uppercase) — it remains the destructive-with-
	// confirm key for the focused session, even when notes are attached.
	r := newRig(t)
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date: today, Start: today.Add(9 * time.Hour), Stop: today.Add(10 * time.Hour),
		Elapsed: time.Hour,
	}
	m := seedSessionAndLoad(t, r, s)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)
	if len(r.sessions.Sessions) != 0 {
		t.Errorf("D + Enter must still delete the focused session, got %d remaining", len(r.sessions.Sessions))
	}
	// Detach must NOT have happened as a side-effect.
	got := r.links.ByDate[r.clock.T.Format("2006-01-02")]
	if len(got) != 1 {
		t.Errorf("D-delete-session must not touch attached notes, got %v", got)
	}
}

// — Woche (wave C) smoke tests —

// loadedWoche drains Init for every sub-model, then switches to the
// Woche tab so View() renders the loaded week. Mirrors loadedHeute but
// with an extra "2" key to land on the Woche tab.
func loadedWoche(t *testing.T, r rig) tea.Model {
	t.Helper()
	updated, _ := r.model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	loaded := drainCmd(t, updated, updated.Init())
	loaded, _ = loaded.Update(tea.KeyPressMsg{Text: "2"})
	return loaded
}

func TestWoche_LoadRendersWeekHeaderAndDayRows(t *testing.T) {
	r := newRig(t)
	m := loadedWoche(t, r)
	out := m.View().Content
	// May 1, 2026 is Friday in ISO week 18.
	if !strings.Contains(out, "KW 18") {
		t.Errorf("Woche header should contain ISO week, got:\n%s", out)
	}
	for _, wd := range []string{"Mo", "Di", "Mi", "Do", "Fr"} {
		if !strings.Contains(out, wd) {
			t.Errorf("Woche should render weekday %q, got:\n%s", wd, out)
		}
	}
	if !strings.Contains(out, "heute") {
		t.Errorf("Friday row should be marked »heute«, got:\n%s", out)
	}
}

func TestWoche_TodayActive_RendersRunningGlyph(t *testing.T) {
	r := newRig(t)
	start := r.clock.T.Add(-30 * time.Minute)
	r.active.Active = &start
	m := loadedWoche(t, r)
	out := m.View().Content
	// "▶" appears both in pace dots (today active) and the row extra; one
	// of either presence is enough proof the running state is wired.
	if !strings.Contains(out, "▶") {
		t.Errorf("running today should add ▶ marker on the Friday row, got:\n%s", out)
	}
}

func TestWoche_DayOff_RendersFeiertagLabel(t *testing.T) {
	r := newRig(t)
	wed := time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local)
	if err := r.dayoffs.Add(domain.DayOff{Date: wed, Kind: domain.KindHoliday, Label: "Tag der Arbeit"}); err != nil {
		t.Fatalf("seed day-off: %v", err)
	}
	m := loadedWoche(t, r)
	out := m.View().Content
	if !strings.Contains(out, "Feiertag") {
		t.Errorf("Wednesday should render »Feiertag«, got:\n%s", out)
	}
}

func TestWoche_CursorJK_TracksStateCursor(t *testing.T) {
	r := newRig(t)
	m := loadedWoche(t, r)
	// Initial cursor is row 0 (Mo).
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("StateCursor before move = %d, want 0", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if got := m.(worktime.Model).StateCursor(); got != 2 {
		t.Errorf("StateCursor after 2× j = %d, want 2", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	if got := m.(worktime.Model).StateCursor(); got != 1 {
		t.Errorf("StateCursor after 2j+1k = %d, want 1", got)
	}
}

func TestWoche_GotoEndsCursor(t *testing.T) {
	r := newRig(t)
	m := loadedWoche(t, r)
	// G jumps to last row. Empty-sessions week renders Mo–Fr (5 rows) for
	// the rig's clock (Friday May 1, 2026), so the last index is 4.
	m, _ = m.Update(tea.KeyPressMsg{Text: "G"})
	if got := m.(worktime.Model).StateCursor(); got != 4 {
		t.Errorf("StateCursor after G = %d, want 4 (Fr)", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "g"})
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("StateCursor after g = %d, want 0", got)
	}
}

func TestWoche_LoadError_RendersErrPath(t *testing.T) {
	r := newRig(t)
	r.sessions.Err = errFake("kaputt")
	m := loadedWoche(t, r)
	out := m.View().Content
	if !strings.Contains(out, "kaputt") {
		t.Errorf("Woche should surface the load error, got:\n%s", out)
	}
}

// — History (wave D) smoke tests —

// loadedHistory drains Init across all sub-models, then switches to the
// History tab so View() renders the loaded records.
func loadedHistory(t *testing.T, r rig) tea.Model {
	t.Helper()
	updated, _ := r.model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	loaded := drainCmd(t, updated, updated.Init())
	loaded, _ = loaded.Update(tea.KeyPressMsg{Text: "3"})
	return loaded
}

// seedHistorySessions plants a handful of past-week sessions (Mo/Tu/We
// of the current ISO week, + one in the previous week) so the list,
// heatmap, tag-clock and month grid all have data.
func seedHistorySessions(r rig) {
	mon := isoMondayOf(r.clock.T)
	mk := func(day time.Time, startH, dur int, tag, note string) domain.Session {
		start := day.Add(time.Duration(startH) * time.Hour)
		stop := start.Add(time.Duration(dur) * time.Hour)
		// Stable deterministic ID per (date, startH) so the ID-based
		// Delete / Upsert paths in SessionWriter find a non-empty key.
		// Real sessions always have a UUID v4; tests just need
		// uniqueness within the seed.
		id := fmt.Sprintf("seed-%s-h%02d", day.Format("20060102"), startH)
		return domain.Session{
			ID:   id,
			Date: day, Start: start, Stop: stop,
			Elapsed: stop.Sub(start), Tag: tag, Note: note,
		}
	}
	r.sessions.Sessions = []domain.Session{
		mk(mon, 9, 4, "deep", "morning standup"),
		mk(mon.AddDate(0, 0, 1), 10, 3, "ops", ""),
		mk(mon.AddDate(0, 0, 2), 9, 2, "deep", "design review"),
		mk(mon.AddDate(0, 0, -7), 14, 2, "ops", ""),
	}
}

func isoMondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).
		AddDate(0, 0, -(wd - 1))
}

func TestHistory_LoadRendersListWithKW(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	out := m.View().Content
	if !strings.Contains(out, "KW ") {
		t.Errorf("history list should contain KW header, got:\n%s", out)
	}
	if !strings.Contains(out, "Tage") || !strings.Contains(out, "Total") {
		t.Errorf("history header should expose Tage/Total volume strip, got:\n%s", out)
	}
}

func TestHistory_LoadEmpty_RendersHint(t *testing.T) {
	r := newRig(t)
	m := loadedHistory(t, r)
	out := m.View().Content
	if !strings.Contains(out, "Keine Treffer") {
		t.Errorf("empty history should hint »Keine Treffer«, got:\n%s", out)
	}
}

func TestHistory_VCyclesModes(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Slice E removed the "Ansicht (Mode)" footer hint (it was showing
	// the current mode rather than the next, see Review-Punkt M5).
	// Probe each mode by a unique body anchor instead.
	steps := []struct {
		name, anchor string
	}{
		{"Heatmap", "█ Ziel"},
		{"Tag-Clock", "≥75%"},
		{"Monat", "Apr 2026"},
		{"Liste", "KW 18 / 2026"},
	}
	for _, want := range steps {
		m, _ = m.Update(tea.KeyPressMsg{Text: "v"})
		got := m.View().Content
		if !strings.Contains(got, want.anchor) {
			t.Errorf("v cycle expected %s mode body anchor %q, got:\n%s", want.name, want.anchor, got)
		}
	}
}

func TestHistory_FilterDialog_TogglesFilterActive(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// "/" opens the filter dialog and FilterActive becomes true.
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("/ should activate the history filter dialog")
	}
	// Tab keys must not switch tabs while a dialog is open.
	m, _ = m.Update(tea.KeyPressMsg{Text: "2"})
	if strings.Contains(m.View().Content, "Woche lädt") {
		t.Error("`2` while filter dialog is open must not switch to Woche")
	}
	// Esc closes the dialog.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(worktime.Model).FilterActive() {
		t.Error("Esc should close the filter dialog")
	}
}

func TestHistory_FilterTag_RendersChip(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, ch := range "tag:deep" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.(worktime.Model).FilterActive() {
		t.Fatal("Enter should commit the filter and close the dialog")
	}
	out := m.View().Content
	if !strings.Contains(out, "filter:") || !strings.Contains(out, "tag:deep") {
		t.Errorf("filter chip should appear in header, got:\n%s", out)
	}
}

func TestHistory_HeatmapNavigates(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, _ = m.Update(tea.KeyPressMsg{Text: "v"}) // → heatmap
	// Move cursor down one row — should still render.
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	out := m.View().Content
	if !strings.Contains(out, "█ Ziel") {
		t.Errorf("heatmap mode expected (legend »█ Ziel«), got:\n%s", out)
	}
	for _, marker := range []string{"Mo", "Di", "So"} {
		if !strings.Contains(out, marker) {
			t.Errorf("heatmap should render weekday rows, missing %q in:\n%s", marker, out)
		}
	}
}

func TestHistory_DrillOpensAndClosesReadOnly(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Enter on the focused list row opens the drill.
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("drill should report FilterActive=true so tab keys don't intercept")
	}
	out := m.View().Content
	// picker.SectionHeader uppercases — match against the case it actually renders.
	if !strings.Contains(strings.ToLower(out), "sessions") || !strings.Contains(out, "→") {
		t.Errorf("drill view should list day's sessions, got:\n%s", out)
	}
	// b dismisses the drill.
	m, _ = m.Update(tea.KeyPressMsg{Text: "b"})
	if m.(worktime.Model).FilterActive() {
		t.Error("`b` should close the drill")
	}
}

func TestHistory_ResetFilterT(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Set a filter, then T resets.
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, ch := range "tag:deep" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Text: "T"})
	out := m.View().Content
	if strings.Contains(out, "filter: ") {
		t.Errorf("T should clear filter, got:\n%s", out)
	}
}

func TestHistory_LoadError_RendersErrPath(t *testing.T) {
	r := newRig(t)
	r.sessions.Err = errFake("kaputt")
	m := loadedHistory(t, r)
	out := m.View().Content
	if !strings.Contains(out, "kaputt") {
		t.Errorf("History should surface the load error, got:\n%s", out)
	}
}

// — Frei (wave E) smoke tests —

// loadedFrei drains Init across all sub-models, then switches to the
// Frei tab so View() renders the loaded entries.
func loadedFrei(t *testing.T, r rig) tea.Model {
	t.Helper()
	updated, _ := r.model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	loaded := drainCmd(t, updated, updated.Init())
	loaded, _ = loaded.Update(tea.KeyPressMsg{Text: "4"})
	return loaded
}

func TestFrei_LoadEmpty_RendersHint(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	out := m.View().Content
	if !strings.Contains(out, "Noch keine Daten") {
		t.Errorf("empty Frei should hint at empty year, got:\n%s", out)
	}
	if !strings.Contains(out, "Frei 2026") {
		t.Errorf("Frei header should expose the year, got:\n%s", out)
	}
}

func TestFrei_LoadEntries_RendersKindAndLabel(t *testing.T) {
	r := newRig(t)
	holiday := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := r.dayoffs.Add(domain.DayOff{
		Date: holiday, Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedFrei(t, r)
	out := m.View().Content
	for _, want := range []string{"Feiertag", "Tag der Arbeit"} {
		if !strings.Contains(out, want) {
			t.Errorf("Frei should render %q, got:\n%s", want, out)
		}
	}
	// picker.SectionHeader uppercases — lower-case the view for the count check.
	if !strings.Contains(strings.ToLower(out), "einträge (1)") {
		t.Errorf("Frei should render einträge count, got:\n%s", out)
	}
}

func TestFrei_QuickAddA_MarksTodayAsVacation(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "A"})
	_ = drainCmd(t, updated, cmd)
	today := r.clock.T.Format("2006-01-02")
	entry, ok := r.dayoffs.Entries[today]
	if !ok {
		t.Fatalf("expected entry for %s after A, got %v", today, r.dayoffs.Entries)
	}
	if entry.Kind != domain.KindVacation {
		t.Errorf("kind = %q, want %q", entry.Kind, domain.KindVacation)
	}
}

func TestFrei_QuickAddK_MarksTodayAsSick(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "K"})
	_ = drainCmd(t, updated, cmd)
	today := r.clock.T.Format("2006-01-02")
	entry, ok := r.dayoffs.Entries[today]
	if !ok {
		t.Fatalf("expected entry for %s after K, got %v", today, r.dayoffs.Entries)
	}
	if entry.Kind != domain.KindSick {
		t.Errorf("kind = %q, want %q", entry.Kind, domain.KindSick)
	}
}

func TestFrei_AddDialog_GatesTabKeys(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, _ = m.Update(tea.KeyPressMsg{Text: "a"})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("`a` should activate the add form")
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "2"})
	if strings.Contains(m.View().Content, "Woche lädt") {
		t.Error("`2` while add dialog is open must not switch to Woche")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(worktime.Model).FilterActive() {
		t.Error("Esc should close the add dialog")
	}
}

func TestFrei_DeleteConfirm_RequiresExplicitConfirm(t *testing.T) {
	r := newRig(t)
	holiday := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := r.dayoffs.Add(domain.DayOff{
		Date: holiday, Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedFrei(t, r)

	// Skill §Keybind grammar: `D` (uppercase) öffnet die destructive Action,
	// confirm.Model: y/Enter → ja, n/Esc → nein.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(r.dayoffs.Entries) != 1 {
		t.Errorf("Esc on delete confirm should cancel, got %d entries", len(r.dayoffs.Entries))
	}

	// Erneut öffnen, mit Enter bestätigen.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "D"})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, updated, cmd)
	if len(r.dayoffs.Entries) != 0 {
		t.Errorf("Enter on delete confirm should delete the entry, got %d remaining", len(r.dayoffs.Entries))
	}
}

func TestFrei_SyncGermanHolidays_PopulatesEntries(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "B"})
	_ = drainCmd(t, updated, cmd)
	if len(r.dayoffs.Entries) == 0 {
		t.Errorf("`B` should populate gesetzliche Feiertage for the displayed year, got 0 entries")
	}
}

func TestFrei_YearNav_ShowsPreviousYearEntries(t *testing.T) {
	r := newRig(t)
	prevYear := time.Date(2025, 12, 25, 0, 0, 0, 0, time.Local)
	if err := r.dayoffs.Add(domain.DayOff{
		Date: prevYear, Kind: domain.KindHoliday, Label: "Weihnachten",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedFrei(t, r)
	if strings.Contains(m.View().Content, "Weihnachten") {
		t.Fatalf("default 2026 view should not show 2025 entry, got:\n%s", m.View().Content)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "h"})
	loaded := drainCmd(t, updated, cmd)
	out := loaded.View().Content
	if !strings.Contains(out, "Weihnachten") {
		t.Errorf("after `h`, 2025 entry should be visible, got:\n%s", out)
	}
	if !strings.Contains(out, "Frei 2025") {
		t.Errorf("after `h`, header should show 2025, got:\n%s", out)
	}
}

// — End-to-end flow (wave F) —

// TestE2E_StartStopTagAppendsLog drives the full action surface a typical
// user touches in a working day: start → stop → tag the resulting
// session. Asserts the fake session store ends up with exactly one row
// whose Tag matches what the user typed. Single integration-style test
// per the wave F plan; the per-feature smoke tests still live above.
func TestE2E_StartStopTagAppendsLog(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)

	// 1. Start.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	m = drainCmd(t, updated, cmd)
	if r.active.Active == nil {
		t.Fatal("after `s` from idle: active marker should be set")
	}

	// 2. Advance the clock so the stop produces a non-zero elapsed.
	r.clock.T = r.clock.T.Add(45 * time.Minute)

	// 3. Stop.
	updated, cmd = m.Update(tea.KeyPressMsg{Text: "s"})
	m = drainCmd(t, updated, cmd)
	if r.active.Active != nil {
		t.Fatal("after `s` while running: active marker should be cleared")
	}
	if got := len(r.sessions.Sessions); got != 1 {
		t.Fatalf("after stop: expected 1 logged session, got %d", got)
	}

	// 4. Open tag dialog, type "deep", commit.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "t"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("`t` on logged session should open the tag dialog")
	}
	for _, ch := range "deep" {
		updated, _ = updated.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	updated, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	final := drainCmd(t, updated, cmd)

	// 5. Assert the log row carries the tag and the dialog closed.
	if final.(worktime.Model).FilterActive() {
		t.Error("tag dialog should be closed after Enter")
	}
	if got := r.sessions.Sessions[0].Tag; got != "deep" {
		t.Errorf("Session.Tag = %q, want %q", got, "deep")
	}
}

// TestHeute_NoteView_FullScreen_BypassesTitlebox pinst den Render-Bug
// aus Round-5b: der `o`-Inline-Viewer rendert ein markdown_overlay mit
// eigenem Frame (rounded border, title, footer, status bar). Der
// worktime-Root packte das vorher in titlebox.Render — Doppel-Border,
// rechte Frame-Spalte abgeschnitten, Tab-Strip oben sichtbar trotz
// Vollbild-Overlay. Der Fix lässt heute via FullScreen() opt-in, dass
// die Sub-Model-View direkt zurueckgegeben wird (analog brief am Root).
func TestHeute_NoteView_FullScreen_BypassesTitlebox(t *testing.T) {
	r := newRig(t)
	r.noteReader.Bodies["daily-2026-05-01"] = "# Daily\n\nhello"
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "o"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	firstLine := out
	if i := strings.Index(out, "\n"); i >= 0 {
		firstLine = out[:i]
	}
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if strings.Contains(firstLine, label) {
			t.Errorf("Note-Viewer Vollbild: erste Zeile darf KEIN Worktime-Tab-Strip enthalten (%q gefunden), got firstLine=%q",
				label, firstLine)
		}
	}
	if !strings.Contains(out, "Note · daily-2026-05-01") {
		t.Errorf("Note-Title sollte sichtbar bleiben, got:\n%s", out)
	}
}

// TestHistory_DrillNoteView_FullScreen_BypassesTitlebox spiegelt den
// Heute-Fall fuer den History-Drill-Viewer (`o` im Drill).
func TestHistory_DrillNoteView_FullScreen_BypassesTitlebox(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "notes/full-screen-check"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Text: "o"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	firstLine := out
	if i := strings.Index(out, "\n"); i >= 0 {
		firstLine = out[:i]
	}
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if strings.Contains(firstLine, label) {
			t.Errorf("Drill-Note-Viewer Vollbild: erste Zeile darf KEIN Worktime-Tab-Strip enthalten (%q gefunden), got firstLine=%q",
				label, firstLine)
		}
	}
	if !strings.Contains(out, "Note · "+preID) {
		t.Errorf("Note-Title sollte sichtbar bleiben, got:\n%s", out)
	}
}

// — small test helpers —

type errFake string

func (e errFake) Error() string { return string(e) }
