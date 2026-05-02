package viewport_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/viewport"
)

func TestInit_ReturnsNoCmd(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 10, testPalette)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init should return nil, got %v", cmd)
	}
}

func TestScrollPercent_AtBottomIs1(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 3, testPalette)
	m.SetContent("a\nb\nc\nd\ne\nf\ng\nh")
	pct := m.ScrollPercent()
	if pct < 0 || pct > 1 {
		t.Errorf("ScrollPercent should be in [0,1], got %f", pct)
	}
}

func TestUpdate_ScrollUpKeyDisablesTail(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 3, testPalette)
	m.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")
	if !m.AtBottom() {
		t.Fatal("precondition: viewport should start at bottom")
	}
	// Send several "up" key messages to scroll away from bottom.
	for i := 0; i < 5; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = updated
	}
	if m.AtBottom() {
		t.Error("expected viewport to be scrolled away from bottom after up keys")
	}
	// New append should NOT auto-jump to bottom now (tail disabled).
	m.AppendContent("\nnew-line")
	if m.AtBottom() {
		t.Error("after scrolling away, AppendContent should not auto-snap to bottom")
	}
}

func TestUpdate_NonKeyMessagesPassThrough(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 10, testPalette)
	m.SetContent("hello")
	// Forward a tea.WindowSizeMsg — should not panic and should not flip tail.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	if !updated.AtBottom() {
		t.Error("non-key message should not flip tail mode away from bottom")
	}
}
