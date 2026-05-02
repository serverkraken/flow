package palette_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type fixture struct {
	entries *testutil.FakePaletteEntryReader
	stats   *testutil.FakePaletteStatsStore
	tmux    *testutil.FakeTmux
	clock   *testutil.FixedClock
}

func newFixture(seed ...domain.PaletteEntry) *fixture {
	return &fixture{
		entries: &testutil.FakePaletteEntryReader{Entries: append([]domain.PaletteEntry(nil), seed...)},
		stats:   &testutil.FakePaletteStatsStore{Stats: domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{}}},
		tmux:    &testutil.FakeTmux{Session: "work"},
		clock:   &testutil.FixedClock{T: time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)},
	}
}

func (f *fixture) model() palette.Model {
	reader := &usecase.PaletteReader{Entries: f.entries, Stats: f.stats, Tmux: f.tmux, Clock: f.clock}
	writer := &usecase.PaletteWriter{Stats: f.stats, Clock: f.clock}
	return palette.New(theme.Load(), reader, writer, f.tmux)
}

// runUntilLoaded runs the Init cmd, applies WindowSizeMsg, applies the
// resulting loadedMsg, and returns the loaded model.
func runUntilLoaded(t *testing.T, m palette.Model) tea.Model {
	t.Helper()
	cmd := m.Init()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(cmd())
	return updated
}

func TestNew_BeforeWindowSize_ViewIsEmpty(t *testing.T) {
	f := newFixture()
	if got := f.model().View(); got != "" {
		t.Errorf("View before WindowSizeMsg should be empty, got %q", got)
	}
}

func TestInit_LoadsAndRendersEntries(t *testing.T) {
	f := newFixture(
		domain.PaletteEntry{Icon: "⚡", Label: "Reload", Action: "source-file ~/.tmux.conf", Section: "System"},
		domain.PaletteEntry{Icon: "★", Label: "Pin demo", Action: "display 'pinned'", Section: "Misc"},
	)
	updated := runUntilLoaded(t, f.model())
	out := updated.View()
	if !strings.Contains(out, "Reload") || !strings.Contains(out, "Pin demo") {
		t.Errorf("View should list both entries, got:\n%s", out)
	}
	if !strings.Contains(out, "session: work") {
		t.Errorf("title should mention tmux session, got:\n%s", out)
	}
	if !strings.Contains(out, "2 Aktionen") {
		t.Errorf("title should report 2 Aktionen, got:\n%s", out)
	}
}

func TestInit_LoadError_DisplaysError(t *testing.T) {
	f := newFixture()
	f.entries.Err = errors.New("plugins gone")
	updated := runUntilLoaded(t, f.model())
	if got := updated.View(); !strings.Contains(got, "plugins gone") {
		t.Errorf("View should surface load error, got:\n%s", got)
	}
}

func TestEnter_DispatchesViaTmux(t *testing.T) {
	f := newFixture(
		domain.PaletteEntry{Label: "Reload", Action: "source-file ~/.tmux.conf", Section: "System"},
	)
	updated := runUntilLoaded(t, f.model())
	updated, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a tea.Cmd")
	}
	msg := cmd()
	if len(f.tmux.Shells) != 1 {
		t.Fatalf("expected 1 RunShell call, got %d", len(f.tmux.Shells))
	}
	if !strings.Contains(f.tmux.Shells[0], "source-file") {
		t.Errorf("RunShell payload should embed the action, got %q", f.tmux.Shells[0])
	}
	// dispatchedMsg → tea.Quit → the next Update produces tea.QuitMsg
	if _, ok := msg.(tea.QuitMsg); !ok {
		_, quit := updated.Update(msg)
		if quit == nil {
			t.Fatal("dispatchedMsg should propagate tea.Quit")
		}
	}
	// Stats: one Mark recorded
	if got := f.stats.Stats.Actions[domain.EntryKey(domain.PaletteEntry{Label: "Reload", Section: "System"})].Count; got != 1 {
		t.Errorf("Mark count: got %d want 1", got)
	}
}

func TestPin_ReloadsAndMovesToFavoriten(t *testing.T) {
	f := newFixture(
		domain.PaletteEntry{Label: "A", Action: "noop A", Section: "System"},
		domain.PaletteEntry{Label: "B", Action: "noop B", Section: "System"},
	)
	updated := runUntilLoaded(t, f.model())
	// Press '.' on entry 0 ("A") to pin
	updated, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
	if cmd == nil {
		t.Fatal("pin should return a load cmd")
	}
	updated, _ = updated.Update(cmd())
	// Persisted state: A pinned
	pinned := f.stats.Stats.Actions[domain.EntryKey(domain.PaletteEntry{Label: "A", Section: "System"})].Pinned
	if !pinned {
		t.Error("A should be pinned in persisted stats")
	}
	out := updated.View()
	if !strings.Contains(strings.ToLower(out), "favoriten") {
		t.Errorf("View after pin should show Favoriten section, got:\n%s", out)
	}
}

func TestSlashFocusesFilter_AppliesFuzzyMatch(t *testing.T) {
	f := newFixture(
		domain.PaletteEntry{Label: "Reload", Action: "x", Section: "System"},
		domain.PaletteEntry{Label: "Pin demo", Action: "y", Section: "Misc"},
	)
	updated := runUntilLoaded(t, f.model())
	// "/" focuses the filter
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !updated.(palette.Model).FilterActive() {
		t.Error("FilterActive should be true after '/'")
	}
	// Type "rel" — narrows to Reload
	for _, r := range "rel" {
		updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	out := updated.View()
	if !strings.Contains(out, "Reload") {
		t.Errorf("filtered view should still show Reload, got:\n%s", out)
	}
	if strings.Contains(out, "Pin demo") {
		t.Errorf("filtered view should hide Pin demo, got:\n%s", out)
	}
}

func TestEsc_QuitsWhenFilterEmpty(t *testing.T) {
	f := newFixture()
	updated := runUntilLoaded(t, f.model())
	_, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should produce tea.Quit")
	}
}

func TestStateRoundtrip(t *testing.T) {
	f := newFixture()
	restored := f.model().WithState("foo", 7)
	m, ok := restored.(palette.Model)
	if !ok {
		t.Fatalf("WithState should return a palette.Model, got %T", restored)
	}
	if m.StateFilter() != "foo" {
		t.Errorf("StateFilter: got %q want foo", m.StateFilter())
	}
	if m.StateCursor() != 7 {
		t.Errorf("StateCursor: got %d want 7", m.StateCursor())
	}
}
