package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

func TestLoadedPopulatesAndViewRenders(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{
		sessions: []domain.WorkSession{run},
		projects: []domain.Project{{ID: "p1", Name: "Flow"}},
		now:      start.Add(25 * time.Minute),
	})
	m = next.(Model)
	if m.running == nil || m.running.ID != "s1" {
		t.Fatal("running session not detected from loaded sessions")
	}
	if !strings.Contains(m.View().Content, "00:25") {
		t.Fatalf("running elapsed not rendered:\n%s", m.View().Content)
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
}

func TestTickAdvancesNow(t *testing.T) {
	m := New(nil, "tester")
	t0 := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	next, _ := m.Update(tickMsg(t0))
	if got := next.(Model).now; !got.Equal(t0) {
		t.Fatalf("tick now = %v", got)
	}
}
