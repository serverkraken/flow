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
		domain.Project{Name: "alpha", Path: "/p/alpha"},
		domain.Project{Name: "beta", Path: "/p/beta"},
		domain.Project{Name: "gamma", Path: "/p/gamma"},
		domain.Project{Name: "delta", Path: "/p/delta"},
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
	if got := m.(projects.Model).StateFilter(); got != "abc" {
		t.Fatalf("filter value: got %q want abc", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.(projects.Model).StateFilter(); got != "ab" {
		t.Errorf("after backspace: got %q want ab", got)
	}
}
