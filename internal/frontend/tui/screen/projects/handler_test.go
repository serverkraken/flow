package projects_test

// Coverage for FilterActive, handleNormalKey navigation, and
// handleFilterKey edges that the existing model_test.go suite leaves
// uncovered.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
)

func makeFixtureWithProjects() *fixture {
	return newFixture(
		domain.SourceDir{Name: "alpha", Path: "/p/alpha"},
		domain.SourceDir{Name: "beta", Path: "/p/beta"},
		domain.SourceDir{Name: "gamma", Path: "/p/gamma"},
		domain.SourceDir{Name: "delta", Path: "/p/delta"},
	)
}

func TestFilterActive_TogglesWithSlashAndEsc(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	if m.(projects.Model).FilterActive() {
		t.Error("default state should not have filter active")
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	if !m.(projects.Model).FilterActive() {
		t.Error("/ should activate filter")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.(projects.Model).FilterActive() {
		t.Error("Esc should deactivate filter")
	}
}

func TestHandleNormalKey_JK(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if got := m.(projects.Model).StateCursor(); got != 1 {
		t.Errorf("after j: cursor 1, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	if got := m.(projects.Model).StateCursor(); got != 0 {
		t.Errorf("after k: cursor 0, got %d", got)
	}
	// k at cursor=0 → no-op (guard branch)
	m, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	if got := m.(projects.Model).StateCursor(); got != 0 {
		t.Errorf("k at top should stay at 0, got %d", got)
	}
}

func TestHandleNormalKey_GAndCapitalG(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "G"})
	if got := m.(projects.Model).StateCursor(); got != 3 {
		t.Errorf("G should jump to last (3), got %d", got)
	}
	// j at last → no-op
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if got := m.(projects.Model).StateCursor(); got != 3 {
		t.Errorf("j at bottom should stay at 3, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Text: "g"})
	if got := m.(projects.Model).StateCursor(); got != 0 {
		t.Errorf("g should jump to 0, got %d", got)
	}
}

func TestHandleNormalKey_PgDownPgUpAndCtrl(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if got := m.(projects.Model).StateCursor(); got != 0 {
		t.Errorf("pgdown then pgup should land at 0, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if got := m.(projects.Model).StateCursor(); got != 0 {
		t.Errorf("ctrl+d then ctrl+u should land at 0, got %d", got)
	}
}

func TestHandleNormalKey_EnterEmpty_NoOp(t *testing.T) {
	f := newFixture() // zero projects
	m := runUntilLoaded(t, f.model())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("enter with no projects should be a no-op, got cmd=%v", cmd)
	}
}

func TestHandleFilterKey_EnterDispatches(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter from filter should dispatch")
	}
	_ = cmd()
}

func TestHandleFilterKey_EnterEmpty_NoOp(t *testing.T) {
	f := newFixture()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("enter from empty filter should not dispatch, got cmd=%v", cmd)
	}
}

func TestHandleFilterKey_BackspaceEditsValue(t *testing.T) {
	f := makeFixtureWithProjects()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
	}
	// StateFilter now encodes tab prefix; sub-filter should contain "abc".
	sf := m.(projects.Model).StateFilter()
	if !containsSubFilter(sf, "abc") {
		t.Fatalf("filter value should contain 'abc', got %q", sf)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	sf = m.(projects.Model).StateFilter()
	if !containsSubFilter(sf, "ab") {
		t.Errorf("after backspace: filter should contain 'ab', got %q", sf)
	}
}

// containsSubFilter checks whether the tab-prefixed filter string
// (format "tab=NAME|<sub>") contains the expected sub-filter value.
// For the Quellverzeichnisse tab, the format is "tab=quellverzeichnisse|<sub>".
func containsSubFilter(stateFilter, want string) bool {
	// Split on "|" and check the second part.
	if idx := len("tab=quellverzeichnisse|"); idx < len(stateFilter) {
		sub := stateFilter[idx:]
		return sub == want
	}
	return stateFilter == want
}

// TestSubTabs verifies the host exposes the correct tab labels.
func TestSubTabs(t *testing.T) {
	f := newFixture()
	tabs := f.model().SubTabs()
	if len(tabs) != 2 {
		t.Fatalf("expected 2 sub-tabs, got %d", len(tabs))
	}
	if tabs[0] != "Quellverzeichnisse" {
		t.Errorf("tab 0 should be 'Quellverzeichnisse', got %q", tabs[0])
	}
	if tabs[1] != "Worktime-Projekte" {
		t.Errorf("tab 1 should be 'Worktime-Projekte', got %q", tabs[1])
	}
}

// TestSwitchSubTab verifies 1/2 key routing via SwitchSubTab.
func TestSwitchSubTab(t *testing.T) {
	f := newFixture()
	m := f.model()
	if m.SubTabIndex() != 0 {
		t.Fatalf("default sub-tab should be 0, got %d", m.SubTabIndex())
	}
	m2, ok := m.SwitchSubTab(1).(projects.Model)
	if !ok {
		t.Fatal("SwitchSubTab should return a projects.Model")
	}
	if m2.SubTabIndex() != 1 {
		t.Errorf("after SwitchSubTab(1): sub-tab should be 1, got %d", m2.SubTabIndex())
	}
	// Out-of-range → no-op.
	m3, ok := m.SwitchSubTab(99).(projects.Model)
	if !ok {
		t.Fatal("SwitchSubTab(99) should return a projects.Model")
	}
	if m3.SubTabIndex() != 0 {
		t.Errorf("SwitchSubTab(99) should be a no-op (tab 0), got %d", m3.SubTabIndex())
	}
}

// TestTabKey_CyclesSubTabs verifies the Tab key switches between sub-tabs.
func TestTabKey_CyclesSubTabs(t *testing.T) {
	f := newFixture()
	m := runUntilLoaded(t, f.model())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.(projects.Model).SubTabIndex(); got != 1 {
		t.Errorf("after Tab: sub-tab should be 1, got %d", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.(projects.Model).SubTabIndex(); got != 0 {
		t.Errorf("after second Tab: sub-tab should wrap to 0, got %d", got)
	}
}
