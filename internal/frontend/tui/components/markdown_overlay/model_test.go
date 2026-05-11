package markdown_overlay_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

// TestNew_HasInitUpdateView pins the bubbletea-style contract: Init
// returns no startup cmd, Update returns the concrete Model (immutable
// update convention), View returns a string. Not strict tea.Model
// because Update returns Model — hosts type-assert via their own
// state field rather than a generic dispatcher.
func TestNew_HasInitUpdateView(t *testing.T) {
	m := markdown_overlay.New(func(src string, _ int) string { return src })
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init: got non-nil cmd, want nil")
	}
	updated, cmd := m.Update(tea.KeyMsg{})
	if cmd != nil {
		t.Errorf("Update on empty model: got cmd %v, want nil", cmd)
	}
	_ = updated.View()
}

func TestSetSource_RendersThroughRenderFunc(t *testing.T) {
	got := ""
	render := func(src string, _ int) string {
		got = src
		return "RENDERED:" + src
	}
	_ = markdown_overlay.New(render,
		markdown_overlay.WithTitle("T"),
		markdown_overlay.WithSource("hello"),
	).SetSize(40, 10)
	if got != "hello" {
		t.Errorf("render input: got %q, want %q", got, "hello")
	}
}

func TestSetSource_ViewContainsRenderedBody(t *testing.T) {
	m := markdown_overlay.New(
		func(src string, _ int) string { return "R:" + src },
		markdown_overlay.WithSource("body"),
	)
	m = m.SetSize(40, 10)
	view := m.View()
	if !strings.Contains(view, "R:body") {
		t.Errorf("view does not contain rendered body. Got:\n%s", view)
	}
}
