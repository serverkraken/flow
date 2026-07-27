package markdown_overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// keyPress builds a KeyPressMsg with the given text. Used only in this file.
func keyPress(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: s}
}

// contains strips ANSI codes and reports whether s contains substr.
func contains(s, substr string) bool {
	return strings.Contains(ansi.Strip(s), substr)
}

func TestCapturesInput_TrueOnlyInSearch(t *testing.T) {
	m := New(func(src string, w int) string { return src }, WithSource("hello"), WithSearch())
	m = m.SetSize(40, 10)
	if m.CapturesInput() {
		t.Fatal("not searching yet: CapturesInput must be false")
	}
	// '/' enters search mode (the overlay's search-launch key).
	m, _ = m.Update(keyPress("/"))
	if !m.CapturesInput() {
		t.Fatal("after '/' the overlay should capture input")
	}
}

func TestRerender_ReflectsRenderFuncChange(t *testing.T) {
	out := "first"
	m := New(func(src string, w int) string { return out }, WithSource("x"))
	m = m.SetSize(40, 10)
	if got := m.View(); !contains(got, "first") {
		t.Fatalf("expected first render:\n%s", got)
	}
	out = "second"
	m = m.Rerender()
	if got := m.View(); !contains(got, "second") {
		t.Fatalf("Rerender should pick up the new RenderFunc output:\n%s", got)
	}
}
