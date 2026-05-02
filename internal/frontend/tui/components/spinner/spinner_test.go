package spinner_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/spinner"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

var testPalette = theme.Load()

func TestNew_ViewContainsLabel(t *testing.T) {
	t.Parallel()
	m := spinner.New("loading…", testPalette)
	if !strings.Contains(m.View(), "loading…") {
		t.Errorf("view missing label: %q", m.View())
	}
}

func TestInit_ReturnsCmd(t *testing.T) {
	t.Parallel()
	m := spinner.New("wait", testPalette)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a tick command")
	}
}

func TestUpdate_NoKeyMsg_NoPanic(t *testing.T) {
	t.Parallel()
	m := spinner.New("test", testPalette)
	m, _ = m.Update(nil)
	_ = m.View()
}
