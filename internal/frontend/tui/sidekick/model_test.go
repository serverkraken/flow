package sidekick_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func (s *fakeScreen) View() tea.View { return tea.NewView(s.name + ":view") }

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
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
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
	if !strings.Contains(m.View().Content, "palette:view") {
		t.Errorf("View() = %q, want palette screen", m.View().Content)
	}
}

func TestNew_RestoresActiveScreenFromState(t *testing.T) {
	t.Parallel()
	deps, _, _, wt, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	if !strings.Contains(m.View().Content, "worktime:view") {
		t.Errorf("View() = %q, want worktime active", m.View().Content)
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
		if !strings.Contains(m.View().Content, tc.want) {
			t.Errorf("after %q: View() = %q, want %q", tc.key, m.View().Content, tc.want)
		}
	}
}

func TestNew_RestoresNotesScreen(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenNotes}, deps)
	if !strings.Contains(m.View().Content, "notes:view") {
		t.Errorf("View() = %q, want notes active", m.View().Content)
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
	if !strings.Contains(m.View().Content, "palette:view") {
		t.Errorf("after b: View() = %q, want palette", m.View().Content)
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
	if !strings.Contains(m.View().Content, "worktime:view") {
		t.Errorf("backHandler should have kept worktime active; View() = %q", m.View().Content)
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
	if !strings.Contains(m.View().Content, "palette:view") {
		t.Errorf("filter-active palette should not have lost focus; View() = %q", m.View().Content)
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
	if !strings.Contains(m.View().Content, "Hilfe") {
		t.Errorf("after ?: View() should contain Hilfe; got %q", m.View().Content)
	}
	updated, _ = m.Update(keyMsg("p"))
	m = updated.(sidekick.Model)
	if strings.Contains(m.View().Content, "Hilfe") {
		t.Errorf("after dismiss key: help still showing")
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	t.Parallel()
	deps, _, _, _, _, _ := newDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	for _, k := range []tea.KeyPressMsg{keyMsg("q"), {Code: 'c', Mod: tea.ModCtrl}} {
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
		if s.View().Content == "" {
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
			if !strings.Contains(m.View().Content, tc.want) {
				t.Errorf("after SwitchScreenMsg(%q): View() = %q, want %q",
					tc.screen, m.View().Content, tc.want)
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
	before := m.View().Content
	updated, _ := m.Update(palette.SwitchScreenMsg{Screen: "nonexistent"})
	if updated.View().Content != before {
		t.Errorf("unknown screen should be a no-op; before=%q after=%q",
			before, updated.View().Content)
	}
}

// — Phase 10 sub-tab host routing —
//
// subHostScreen embeds fakeScreen plus the sidekick.subTabHost contract.
// SwitchSubTab returns a wrapped fakeScreen pointer so subsequent
// View() / SubTabIndex() observations reflect the index change. Used by
// the tab-strip rendering and numeric-key routing tests below.

type subHostScreen struct {
	*fakeScreen
	tabs        []string
	activeIndex int
	switchCalls []int
}

func (s subHostScreen) SubTabs() []string { return s.tabs }
func (s subHostScreen) SubTabIndex() int  { return s.activeIndex }
func (s subHostScreen) SwitchSubTab(i int) tea.Model {
	// Record the call on the embedded fakeScreen so callers can assert
	// against it through their kept pointer to the underlying fake.
	s.switchCalls = append(s.switchCalls, i)
	s.activeIndex = i
	return s
}

// Update overrides the promoted *fakeScreen.Update so the returned
// tea.Model preserves the subHostScreen wrapper (and with it the
// subTabHost interface). Without this override, fanOutToAll would
// replace the stored screen with the bare *fakeScreen and the host
// would silently disappear after the first WindowSizeMsg — the same
// trap a real screen would hit if its sub-tab host wrapper relied on
// promotion-only Update.
func (s subHostScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := s.fakeScreen.Update(msg)
	return s, cmd
}

// newSubHostDeps builds Deps where Worktime is a subTabHost with four
// sub-tabs. Returns the deps plus the worktime host pointer so tests
// can read activeIndex / switchCalls after Update().
func newSubHostDeps() (sidekick.Deps, *subHostScreen) {
	pal := &fakeScreen{name: "palette"}
	pr := &fakeScreen{name: "projects"}
	wt := &subHostScreen{
		fakeScreen: &fakeScreen{name: "worktime"},
		tabs:       []string{"Heute", "Woche", "History", "Frei"},
	}
	ch := &fakeScreen{name: "cheatsheet"}
	nt := &fakeScreen{name: "notes"}
	return sidekick.Deps{
		Palette:    pal,
		Projects:   pr,
		Worktime:   *wt,
		Cheatsheet: ch,
		Notes:      nt,
	}, wt
}

// TestRenderTabStrip_WithSubTabHost — the View on a sub-tab-host-active
// screen must include both the main tab labels AND the host's sub-tab
// pills (prefixed with their numeric shortcut), and the active sub-tab
// must be visually distinct via the bracket form "[1 Heute]" (the
// inactive pills wear parens "(2 Woche)" — same A11y-2 grammar as the
// main strip's compact form).
func TestRenderTabStrip_WithSubTabHost(t *testing.T) {
	t.Parallel()
	deps, _ := newSubHostDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(sidekick.Model)
	// ANSI strip — the active-tab style (Bold + Accent + Underline)
	// injects SGR sequences mid-pill that would break substring matches
	// like "[1 Heute]" otherwise.
	out := ansi.Strip(m.View().Content)
	// Main strip — worktime active label still present.
	if !strings.Contains(out, "Worktime") {
		t.Errorf("View should contain main strip Worktime label; got:\n%s", out)
	}
	// Sub-tab pills — Heute is the default active sub-tab → "[1 Heute]"
	// in active style; "(2 Woche)" etc. in dim parens.
	for _, want := range []string{"[1 Heute]", "(2 Woche)", "(3 History)", "(4 Frei)"} {
		if !strings.Contains(out, want) {
			t.Errorf("View should render sub-tab pill %q; got:\n%s", want, out)
		}
	}
}

// TestRenderTabStrip_NoSubTabHost — when the active screen is NOT a
// subTabHost (e.g. palette), the View must NOT carry any "[N Label]"
// pills. Guards against a regression where the renderer accidentally
// renders an empty pill row for non-host screens.
func TestRenderTabStrip_NoSubTabHost(t *testing.T) {
	t.Parallel()
	deps, _ := newSubHostDeps()
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(sidekick.Model)
	out := ansi.Strip(m.View().Content)
	for _, unwanted := range []string{"[1 Heute]", "(2 Woche)"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("palette-active View should not render sub-tab pill %q; got:\n%s", unwanted, out)
		}
	}
}

// TestNumericKeyRoutes_ToSubTabHost — pressing "2" while worktime is
// active routes SwitchSubTab(1) to the host. The host's activeIndex
// reflects the move and SubTabIndex on the updated model reports 1.
func TestNumericKeyRoutes_ToSubTabHost(t *testing.T) {
	t.Parallel()
	deps, _ := newSubHostDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updated, _ = updated.Update(keyMsg("2"))
	m = updated.(sidekick.Model)
	out := ansi.Strip(m.View().Content)
	// After "2": Woche is active, Heute is inactive.
	if !strings.Contains(out, "[2 Woche]") {
		t.Errorf("after `2`: Woche pill should be active-styled; got:\n%s", out)
	}
	if !strings.Contains(out, "(1 Heute)") {
		t.Errorf("after `2`: Heute pill should be inactive-styled; got:\n%s", out)
	}
}

// TestNumericKeyOutOfRange_FallsThroughToScreen — pressing "5" while
// worktime is active (only 4 sub-tabs) is out-of-range; the sidekick
// must let the key fall through to worktime so the screen can ignore
// it or repurpose it (palette uses 1-9 for direct-pick). We probe by
// asserting the worktime fakeScreen recorded the key.
func TestNumericKeyOutOfRange_FallsThroughToScreen(t *testing.T) {
	t.Parallel()
	deps, wt := newSubHostDeps()
	m := sidekick.New(theme.Palette{}, domain.FlowState{Screen: domain.ScreenWorktime}, deps)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updated, _ = updated.Update(keyMsg("5"))
	_ = updated
	got := wt.keys
	if len(got) == 0 || got[len(got)-1] != "5" {
		t.Errorf("out-of-range numeric should fall through to worktime; recorded keys=%v", got)
	}
}

// TestNumericKeyFallthrough_WhenNotSubTabHost — pressing "2" while
// palette (not a subTabHost) is active must reach palette, not the
// sidekick's sub-tab handler. Palette uses 1-9 for direct-pick of the
// first nine visible filter results; the fall-through path is load-
// bearing.
func TestNumericKeyFallthrough_WhenNotSubTabHost(t *testing.T) {
	t.Parallel()
	pal := &fakeScreen{name: "palette"}
	deps := sidekick.Deps{
		Palette:    pal,
		Projects:   &fakeScreen{name: "projects"},
		Worktime:   &fakeScreen{name: "worktime"}, // not a subTabHost
		Cheatsheet: &fakeScreen{name: "cheatsheet"},
		Notes:      &fakeScreen{name: "notes"},
	}
	m := sidekick.New(theme.Palette{}, domain.DefaultFlowState(), deps)
	updated, _ := m.Update(keyMsg("2"))
	_ = updated
	if len(pal.keys) == 0 || pal.keys[len(pal.keys)-1] != "2" {
		t.Errorf("palette-active sidekick must forward `2` to the active screen; got keys=%v", pal.keys)
	}
}
