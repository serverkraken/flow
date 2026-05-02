package toast_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

var testPalette = theme.Load()

func TestNew_VisibleInitially(t *testing.T) {
	t.Parallel()
	m := toast.New("saved", 3*time.Second, testPalette)
	if !m.Visible() {
		t.Error("toast should be visible after creation")
	}
}

func TestView_ContainsText(t *testing.T) {
	t.Parallel()
	m := toast.New("Branch erstellt", 3*time.Second, testPalette)
	if !strings.Contains(m.View(), "Branch erstellt") {
		t.Error("view missing toast text")
	}
}

func TestUpdate_DismissedMsg_HidesToast(t *testing.T) {
	t.Parallel()
	m := toast.New("done", 1*time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if m.Visible() {
		t.Error("toast should be hidden after DismissedMsg")
	}
}

func TestView_EmptyAfterDismiss(t *testing.T) {
	t.Parallel()
	m := toast.New("done", 1*time.Second, testPalette)
	m, _ = m.Update(toast.DismissedMsg{})
	if m.View() != "" {
		t.Errorf("expected empty view after dismiss, got %q", m.View())
	}
}

func TestInit_ReturnsCmd(t *testing.T) {
	t.Parallel()
	m := toast.New("hi", 2*time.Second, testPalette)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a tick command")
	}
}
