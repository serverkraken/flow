package viewport_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/components/viewport"
)

var testPalette = theme.Load()

func TestSetContent_ViewContainsText(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 10, testPalette)
	m.SetContent("hello world")
	if !strings.Contains(m.View(), "hello world") {
		t.Errorf("view missing content: %q", m.View())
	}
}

func TestAppendContent_AccumulatesText(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 10, testPalette)
	m.SetContent("line1\n")
	m.AppendContent("line2\n")
	if !strings.Contains(m.Content(), "line1") {
		t.Error("content missing line1")
	}
	if !strings.Contains(m.Content(), "line2") {
		t.Error("content missing line2")
	}
}

func TestSetContent_ReplacesExisting(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 10, testPalette)
	m.SetContent("first")
	m.SetContent("second")
	if strings.Contains(m.Content(), "first") {
		t.Error("old content should be replaced")
	}
	if !strings.Contains(m.Content(), "second") {
		t.Error("new content missing")
	}
}

func TestSetSize_UpdatesDimensions(t *testing.T) {
	t.Parallel()
	m := viewport.New(20, 5, testPalette)
	m.SetSize(80, 24)
	m.SetContent("test")
	out := m.View()
	if out == "" {
		t.Error("expected non-empty view after resize")
	}
}

func TestNew_TailEnabledByDefault(t *testing.T) {
	t.Parallel()
	m := viewport.New(40, 3, testPalette)
	m.SetTail(false)
	m.SetTail(true)
	m.SetContent("a\nb\nc\nd\ne")
	if !m.AtBottom() {
		t.Error("expected to be at bottom with tail enabled")
	}
}
