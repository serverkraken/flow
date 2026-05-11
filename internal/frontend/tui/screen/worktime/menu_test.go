// White-box tests for the worktime action menu's internal state.
// Public-API tests (`:` key opens, FilterActive reflects, etc.) live
// in model_test.go alongside the rest of the Model contract.

package worktime

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pal is the canonical Tokyonight Storm palette used across menu tests.
// theme.Load reads tmux user-options at runtime; in a test process those
// aren't set, so Load falls back to the documented hex values.
func pal() theme.Palette { return theme.Load() }

// rune-press synthesises a single-rune KeyMsg the way bubbletea
// forwards them out of a real TTY.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func keyName(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// menuFor constructs a menu against an empty Deps so predicates that
// touch deps return false (no Reader → "Korrektur" stays hidden).
// Tests that care about predicate results override Deps explicitly.
func menuFor(t *testing.T, activeTab tab) menuModel {
	t.Helper()
	m := newMenuModel(pal(), Deps{})
	m = m.SetSize(120, 40)
	return m.openMenu(activeTab)
}

func TestMenu_OpenSetsActiveAndFiltersByTab(t *testing.T) {
	m := menuFor(t, tabFrei)
	if !m.Active() {
		t.Fatal("openMenu must set Active()")
	}
	// Korrektur is Heute-only and should not show on Frei. Land for
	// Feiertage is general and must show on every tab.
	wantAbsent := "Startzeit der laufenden Session korrigieren"
	wantPresent := "Land für Feiertage"
	var hasLand, hasCorrect bool
	for _, a := range m.filtered {
		switch a.label {
		case wantPresent:
			hasLand = true
		case wantAbsent:
			hasCorrect = true
		}
	}
	if !hasLand {
		t.Errorf("Frei tab should expose %q in menu", wantPresent)
	}
	if hasCorrect {
		t.Errorf("Frei tab must hide %q (Heute-only)", wantAbsent)
	}
}

func TestMenu_GeneralActionsAlwaysPresent(t *testing.T) {
	m := menuFor(t, tabHeute)
	for _, want := range []string{
		"Brief Wochenbericht",
		"Brief Monatsbericht",
		"Export CSV",
		"Export JSON",
		"Stats für Range",
		"Land für Feiertage",
	} {
		var found bool
		for _, a := range m.filtered {
			if a.label == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("general action missing: %q", want)
		}
	}
}

func TestMenu_NavigationWrapsAndJumps(t *testing.T) {
	m := menuFor(t, tabHeute)
	n := len(m.filtered)
	if n < 2 {
		t.Fatalf("need >= 2 actions for nav test; got %d", n)
	}
	// j down once
	m, _ = m.handleKey(keyName("j"))
	if m.cursor != 1 {
		t.Errorf("after j, cursor = %d, want 1", m.cursor)
	}
	// k up wraps
	m, _ = m.handleKey(keyName("k"))
	m, _ = m.handleKey(keyName("k"))
	if m.cursor != n-1 {
		t.Errorf("after kk from 1, cursor = %d, want %d (wrap)", m.cursor, n-1)
	}
	// G jumps to last
	m.cursor = 0
	m, _ = m.handleKey(runeKey('G'))
	if m.cursor != n-1 {
		t.Errorf("after G, cursor = %d, want %d", m.cursor, n-1)
	}
	// g jumps to first
	m, _ = m.handleKey(runeKey('g'))
	if m.cursor != 0 {
		t.Errorf("after g, cursor = %d, want 0", m.cursor)
	}
}

func TestMenu_LiveFilterNarrowsList(t *testing.T) {
	m := menuFor(t, tabHeute)
	full := len(m.filtered)
	// Type "csv" — only Export CSV should remain.
	for _, r := range "csv" {
		m, _ = m.handleKey(runeKey(r))
	}
	if len(m.filtered) >= full {
		t.Errorf("after typing csv, list size %d should shrink from %d", len(m.filtered), full)
	}
	if len(m.filtered) != 1 || m.filtered[0].label != "Export CSV" {
		t.Errorf("filter csv → got %d entries (%v), want 1× Export CSV",
			len(m.filtered), labels(m.filtered))
	}
	// Backspace once — query becomes "cs"; CSV still matches.
	m, _ = m.handleKey(keyName("backspace"))
	if m.query != "cs" {
		t.Errorf("query after backspace = %q, want cs", m.query)
	}
}

func TestMenu_EscClearsQueryFirstThenCloses(t *testing.T) {
	m := menuFor(t, tabHeute)
	for _, r := range "brief" {
		m, _ = m.handleKey(runeKey(r))
	}
	if m.query == "" {
		t.Fatal("precondition: query must be non-empty")
	}
	// 1st esc clears query
	m, _ = m.handleKey(keyName("esc"))
	if m.query != "" {
		t.Errorf("1st esc should clear query; got %q", m.query)
	}
	if !m.Active() {
		t.Error("1st esc must NOT close the menu")
	}
	// 2nd esc closes
	m, _ = m.handleKey(keyName("esc"))
	if m.Active() {
		t.Error("2nd esc must close the menu")
	}
}

func TestMenu_NavRunesNotConsumedByFilter(t *testing.T) {
	m := menuFor(t, tabHeute)
	// Typing j should advance cursor, not extend the query.
	m, _ = m.handleKey(runeKey('j'))
	if m.query != "" {
		t.Errorf("j must navigate, not filter; query = %q", m.query)
	}
	if m.cursor != 1 {
		t.Errorf("j must advance cursor; got %d", m.cursor)
	}
}

func TestComputeMenuActions_EmptyQueryReturnsAllVisible(t *testing.T) {
	all := computeMenuActions(menuContext{activeTab: tabHeute}, "", "")
	if len(all) == 0 {
		t.Fatal("empty query must return at least the general actions")
	}
}

// labels is a debug helper for filter assertions: returns the .label
// of every menuAction in s as a flat slice.
func labels(s []menuAction) []string {
	out := make([]string, 0, len(s))
	for _, a := range s {
		out = append(out, a.label)
	}
	return out
}

func TestMenu_ViewWithoutSizeIsEmpty(t *testing.T) {
	m := newMenuModel(pal(), Deps{})
	m = m.openMenu(tabHeute)
	if got := m.View(); got != "" {
		t.Errorf("View before SetSize must be empty; got %q", got)
	}
}

func TestMenu_ViewClosedIsEmpty(t *testing.T) {
	m := newMenuModel(pal(), Deps{}).SetSize(100, 30)
	if got := m.View(); got != "" {
		t.Errorf("View when closed must be empty; got %q", got)
	}
}

func TestMenu_ViewRendersInjectedToast(t *testing.T) {
	m := menuFor(t, tabHeute).SetSize(120, 40)
	tt := toast.NewDefault("hi", m.pal)
	m.toast = &tt
	out := m.View()
	if !strings.Contains(out, "hi") {
		t.Errorf("View must surface m.toast; got:\n%s", out)
	}
}

func TestMenu_ViewRendersErrorMessage(t *testing.T) {
	m := menuFor(t, tabHeute)
	m.errMsg = "fake error"
	out := m.View()
	if !strings.Contains(out, "fake error") {
		t.Errorf("View must render m.errMsg; got:\n%s", out)
	}
}

func TestMenu_ViewWithEmptyFilterShowsHint(t *testing.T) {
	m := menuFor(t, tabHeute)
	// Filter that matches nothing.
	for _, r := range "zzznomatch" {
		m, _ = m.handleKey(runeKey(r))
	}
	out := m.View()
	if !strings.Contains(out, "Keine Aktion") {
		t.Errorf("empty-filter View must hint »Keine Aktion«; got:\n%s", out)
	}
}

func TestMenu_UpdateClearsToastOnDismiss(t *testing.T) {
	m := menuFor(t, tabHeute)
	tt := toast.NewDefault("transient", m.pal)
	m.toast = &tt
	m, _ = m.Update(toast.DismissedMsg{})
	if m.toast != nil {
		t.Error("toast.DismissedMsg should clear the toast")
	}
}

func TestMenu_BackspaceOnEmptyQueryIsNoop(t *testing.T) {
	m := menuFor(t, tabHeute)
	if m.query != "" {
		t.Fatal("precondition: query must be empty")
	}
	m, _ = m.handleKey(keyName("backspace"))
	if m.query != "" {
		t.Errorf("backspace on empty query must be a no-op; got %q", m.query)
	}
}

// landOrDefault replaced currentLand() once env-resolution moved to
// the composition root (review finding A1). The contract is the same:
// pass-through for non-empty values, "NW" baseline otherwise.
func TestLandOrDefault_PassesThroughExplicitValue(t *testing.T) {
	if got := landOrDefault("BY"); got != "BY" {
		t.Errorf("landOrDefault(BY) = %q, want BY", got)
	}
}

func TestLandOrDefault_DefaultsToNW(t *testing.T) {
	if got := landOrDefault(""); got != "NW" {
		t.Errorf("landOrDefault(\"\") = %q, want NW", got)
	}
}
