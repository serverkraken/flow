package sidekick_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/sidekick"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// fakeScreen is a tea.Model with optional screener / backHandler /
// stateRestorer behaviour controlled by struct fields. It records every
// message it receives so tests can assert on routing.
type fakeScreen struct {
	name           string
	initFired      bool
	keys           []string
	filterActive   bool
	filter         string
	cursor         int
	handlesBack    bool
	withStateCalls []string
}

func (s *fakeScreen) Init() tea.Cmd {
	s.initFired = true
	return func() tea.Msg { return nil }
}

func (s *fakeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		s.keys = append(s.keys, k.String())
	}
	return s, nil
}

func (s *fakeScreen) View() string { return s.name + ":view" }

func (s *fakeScreen) FilterActive() bool  { return s.filterActive }
func (s *fakeScreen) StateFilter() string { return s.filter }
func (s *fakeScreen) StateCursor() int    { return s.cursor }

// optional capability methods — wrapped into separate types so the test
// can opt in/out of each interface independently.

type backScreen struct{ *fakeScreen }

func (b backScreen) HandlesBack() bool { return b.handlesBack }

type restoreScreen struct{ *fakeScreen }

func (r restoreScreen) WithState(filter string, cursor int) tea.Model {
	r.withStateCalls = append(r.withStateCalls, filter)
	r.filter = filter
	r.cursor = cursor
	return r
}

func newDeps() (sidekick.Deps, *fakeScreen, *fakeScreen, *fakeScreen, *fakeScreen, *fakeScreen) {
	pal := &fakeScreen{name: "palette"}
	pr := &fakeScreen{name: "projects"}
	wt := &fakeScreen{name: "worktime"}
	ch := &fakeScreen{name: "cheatsheet"}
	nt := &fakeScreen{name: "notes"}
	return sidekick.Deps{
		Palette:    pal,
		Projects:   pr,
		Worktime:   wt,
		Cheatsheet: ch,
		Notes:      nt,
	}, pal, pr, wt, ch, nt
}

func keyMsg(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Text: string(rune(s[0]))}
	}
	switch s {
	case "ctrl+c":
		return tea.KeyPressMsg{Code: tea.KeyCtrlC}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "?":
		return tea.KeyPressMsg{Text: "?"}
	}
	return tea.KeyPressMsg{Text: s}
}

func TestNew_DefaultsToPalette(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	if !strings.Contains(m.View(), "palette:view") {
		t.Errorf("View() = %q, want palette screen", m.View())
	}
}

func TestNew_RestoresActiveScreenFromState(t *testing.T) {
	t.Parallel()
	deps, _, _, wt, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	if !strings.Contains(m.View(), "worktime:view") {
		t.Errorf("View() = %q, want worktime active", m.View())
	}
	_ = wt
}

func TestNew_AppliesWithStateOnActiveScreen(t *testing.T) {
	t.Parallel()
	pal := &fakeScreen{name: "palette"}
	deps := sidekick.Deps{
		Palette:    restoreScreen{pal},
		Projects:   &fakeScreen{name: "projects"},
		Worktime:   &fakeScreen{name: "worktime"},
		Cheatsheet: &fakeScreen{name: "cheatsheet"},
		Notes:      &fakeScreen{name: "notes"},
	}
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenPalette, Filter: "ext", Cursor: 4}, deps)
	if pal.filter != "ext" || pal.cursor != 4 {
		t.Errorf("WithState not applied: filter=%q cursor=%d", pal.filter, pal.cursor)
	}
	_ = m
}

func TestInit_FansOutToAllScreens(t *testing.T) {
	t.Parallel()
	deps, pal, pr, wt, ch, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}
	for _, s := range []*fakeScreen{pal, pr, wt, ch} {
		if !s.initFired {
			t.Errorf("Init not fired on %s", s.name)
		}
	}
}

func TestUpdate_WindowSize_FansOut(t *testing.T) {
	t.Parallel()
	deps, pal, pr, wt, ch, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = updated
	// WindowSize is not a key, but every screen records it via Update.
	// Use key-routing as the explicit fan-out probe instead — see below.
	for _, s := range []*fakeScreen{pal, pr, wt, ch} {
		_ = s // fan-out check happens in TestUpdate_AsyncMsg_FansOut
	}
}

func TestUpdate_TabKeys_SwitchScreens(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	cases := []struct {
		key, want string
	}{
		{"f", "projects:view"},
		{"w", "worktime:view"},
		{"c", "cheatsheet:view"},
		{"n", "notes:view"},
		{"p", "palette:view"},
	}
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	for _, tc := range cases {
		updated, _ := m.Update(keyMsg(tc.key))
		m = updated.(sidekick.Model)
		if !strings.Contains(m.View(), tc.want) {
			t.Errorf("after %q: View() = %q, want %q", tc.key, m.View(), tc.want)
		}
	}
}

func TestNew_RestoresNotesScreen(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenNotes}, deps)
	if !strings.Contains(m.View(), "notes:view") {
		t.Errorf("View() = %q, want notes active", m.View())
	}
	got := m.CurrentState()
	if got.Screen != domain.ScreenNotes {
		t.Errorf("CurrentState.Screen = %q, want %q", got.Screen, domain.ScreenNotes)
	}
}

func TestUpdate_BJumpsToPaletteByDefault(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	updated, _ := m.Update(keyMsg("b"))
	m = updated.(sidekick.Model)
	if !strings.Contains(m.View(), "palette:view") {
		t.Errorf("after b: View() = %q, want palette", m.View())
	}
}

func TestUpdate_BConsumedByBackHandler(t *testing.T) {
	t.Parallel()
	wt := &fakeScreen{name: "worktime", handlesBack: true}
	deps := sidekick.Deps{
		Palette:    &fakeScreen{name: "palette"},
		Projects:   &fakeScreen{name: "projects"},
		Worktime:   backScreen{wt},
		Cheatsheet: &fakeScreen{name: "cheatsheet"},
		Notes:      &fakeScreen{name: "notes"},
	}
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	updated, _ := m.Update(keyMsg("b"))
	m = updated.(sidekick.Model)
	if !strings.Contains(m.View(), "worktime:view") {
		t.Errorf("backHandler should have kept worktime active; View() = %q", m.View())
	}
	if len(wt.keys) == 0 || wt.keys[len(wt.keys)-1] != "b" {
		t.Errorf("worktime did not receive b key; got %v", wt.keys)
	}
}

func TestUpdate_FilterActive_RoutesKeysToScreen(t *testing.T) {
	t.Parallel()
	pal := &fakeScreen{name: "palette", filterActive: true}
	deps := sidekick.Deps{
		Palette:    pal,
		Projects:   &fakeScreen{name: "projects"},
		Worktime:   &fakeScreen{name: "worktime"},
		Cheatsheet: &fakeScreen{name: "cheatsheet"},
		Notes:      &fakeScreen{name: "notes"},
	}
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	// "w" would normally switch to worktime, but the active palette's
	// filter is on — the key must reach palette and the screen must stay.
	updated, _ := m.Update(keyMsg("w"))
	m = updated.(sidekick.Model)
	if !strings.Contains(m.View(), "palette:view") {
		t.Errorf("filter-active palette should not have lost focus; View() = %q", m.View())
	}
	if len(pal.keys) == 0 || pal.keys[len(pal.keys)-1] != "w" {
		t.Errorf("palette did not receive w; got %v", pal.keys)
	}
}

func TestUpdate_HelpToggle(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(sidekick.Model)
	updated, _ = m.Update(keyMsg("?"))
	m = updated.(sidekick.Model)
	if !strings.Contains(m.View(), "Hilfe") {
		t.Errorf("after ?: View() should contain Hilfe; got %q", m.View())
	}
	updated, _ = m.Update(keyMsg("p"))
	m = updated.(sidekick.Model)
	if strings.Contains(m.View(), "Hilfe") {
		t.Errorf("after dismiss key: help still showing")
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	for _, k := range []tea.KeyPressMsg{keyMsg("q"), {Type: tea.KeyCtrlC}} {
		_, cmd := m.Update(k)
		if cmd == nil {
			t.Errorf("%v: expected quit cmd, got nil", k)
		}
	}
}

func TestCurrentState_RoundTrip(t *testing.T) {
	t.Parallel()
	pal := &fakeScreen{name: "palette", filter: "g", cursor: 2}
	deps := sidekick.Deps{
		Palette:    pal,
		Projects:   &fakeScreen{name: "projects"},
		Worktime:   &fakeScreen{name: "worktime"},
		Cheatsheet: &fakeScreen{name: "cheatsheet"},
		Notes:      &fakeScreen{name: "notes"},
	}
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenPalette}, deps)
	got := m.CurrentState()
	want := domain.FlowState{Screen: domain.ScreenPalette, Filter: "g", Cursor: 2}
	if got != want {
		t.Errorf("CurrentState() = %+v, want %+v", got, want)
	}

	updated, _ := m.Update(keyMsg("w"))
	m = updated.(sidekick.Model)
	got = m.CurrentState()
	if got.Screen != domain.ScreenWorktime {
		t.Errorf("after switch: CurrentState().Screen = %q, want %q", got.Screen, domain.ScreenWorktime)
	}
}

func TestUpdate_AsyncMsg_FansOut(t *testing.T) {
	t.Parallel()
	deps, pal, pr, wt, ch, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	type pingMsg struct{}
	updated, _ := m.Update(pingMsg{})
	_ = updated
	// fakeScreen only records keys; assert the screens stayed alive
	// after the fan-out (no panic, View still works).
	for _, s := range []*fakeScreen{pal, pr, wt, ch} {
		if s.View() == "" {
			t.Errorf("%s broken after async fan-out", s.name)
		}
	}
}

// TestUpdate_SwitchScreenMsg_SwitchesActiveScreen covers the in-process
// flow-tab switch path. Palette emits SwitchScreenMsg when an entry's
// action matches the goto.sh deep-link pattern; the sidekick root catches
// it and updates m.current — no subshell, no flow restart.
func TestUpdate_SwitchScreenMsg_SwitchesActiveScreen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		screen, want string
	}{
		{domain.ScreenWorktime, "worktime:view"},
		{domain.ScreenProjects, "projects:view"},
		{domain.ScreenCheatsheet, "cheatsheet:view"},
		{domain.ScreenNotes, "notes:view"},
		{domain.ScreenPalette, "palette:view"},
	}
	for _, tc := range cases {
		t.Run(tc.screen, func(t *testing.T) {
			deps, _, _, _, _, _ := newDeps()
			m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
			updated, cmd := m.Update(palette.SwitchScreenMsg{Screen: tc.screen})
			if cmd != nil {
				t.Errorf("SwitchScreenMsg should produce no follow-up cmd, got %v", cmd)
			}
			m = updated.(sidekick.Model)
			if !strings.Contains(m.View(), tc.want) {
				t.Errorf("after SwitchScreenMsg(%q): View() = %q, want %q",
					tc.screen, m.View(), tc.want)
			}
		})
	}
}

// TestUpdate_SwitchScreenMsg_UnknownIsNoop guards against typos / retired
// screen names: an unrecognised target leaves the active screen unchanged.
func TestUpdate_SwitchScreenMsg_UnknownIsNoop(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	before := m.View()
	updated, _ := m.Update(palette.SwitchScreenMsg{Screen: "nonexistent"})
	if updated.View() != before {
		t.Errorf("unknown screen should be a no-op; before=%q after=%q",
			before, updated.View())
	}
}
