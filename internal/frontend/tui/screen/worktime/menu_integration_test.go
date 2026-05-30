// Public-API tests for the worktime action menu — exercises the
// integration between Worktime root and menuModel through the
// exported Model interface (`:` opens, FilterActive reflects,
// ConsumesKeys, View overlays).
//
// White-box menu tests (live filter, navigation wraps, predicate
// filtering, runAction toast) live next to menuModel in menu_test.go.

package worktime_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

func TestMenu_ColonKeyOpensActionsModal(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if updated.(worktime.Model).FilterActive() {
		t.Fatal("precondition: menu must be closed before `:`")
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Text: ":"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("`:` must open the menu (FilterActive should be true)")
	}
	out := ansi.Strip(updated.View().Content)
	for _, want := range []string{"Aktionen", "Brief Wochenbericht", "Export CSV"} {
		if !strings.Contains(out, want) {
			t.Errorf("menu View must include %q, got:\n%s", want, out)
		}
	}
}

func TestMenu_TabStripStaysVisibleWhenMenuOpen(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: ":"})
	out := ansi.Strip(updated.View().Content)
	for _, label := range []string{"Heute", "Woche", "Verlauf", "Frei"} {
		if !strings.Contains(out, label) {
			t.Errorf("tab strip must remain visible while menu is open; missing %q:\n%s",
				label, out)
		}
	}
}

func TestMenu_EscClosesAndRestoresTab(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: ":"})
	if !updated.(worktime.Model).FilterActive() {
		t.Fatal("precondition: menu must be open")
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if updated.(worktime.Model).FilterActive() {
		t.Error("Esc must close the menu")
	}
	// After close, the active tab body should render again. Heute is
	// the default tab and shows the loading placeholder before Init.
	if got := ansi.Strip(updated.View().Content); !strings.Contains(got, "Heute lädt") {
		t.Errorf("after Esc, tab body must render again; got:\n%s", got)
	}
}

func TestMenu_TabSwitchKeysSuspendedWhileMenuOpen(t *testing.T) {
	m := newModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: ":"})
	// `2` would normally jump to the Woche tab. While the menu is
	// open the keystroke must go to the menu's filter (extending
	// query) rather than switching tabs.
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "2"})
	out := ansi.Strip(updated.View().Content)
	if strings.Contains(out, "Woche lädt") {
		t.Errorf("`2` while menu open must NOT switch to Woche; got:\n%s", out)
	}
}

func TestMenu_ConsumesKeysIncludesColon(t *testing.T) {
	m := newModel(t)
	keys := m.ConsumesKeys()
	var found bool
	for _, k := range keys {
		if k == ":" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ConsumesKeys must include `:` so sidekick lets it through; got %v", keys)
	}
}
