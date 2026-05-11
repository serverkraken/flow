package markdown_overlay_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

func TestView_ChromeContainsTitleAndBody(t *testing.T) {
	m := markdown_overlay.New(
		func(_ string, _ int) string { return "BODY" },
		markdown_overlay.WithTitle("MyTitle"),
		markdown_overlay.WithSource("x"),
	).SetSize(40, 10)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "MyTitle") {
		t.Errorf("title missing from view:\n%s", out)
	}
	if !strings.Contains(out, "BODY") {
		t.Errorf("body missing from view:\n%s", out)
	}
}

func TestView_StatusBarShowsScrollPercent(t *testing.T) {
	// Multi-line body wider than viewport-height forces scroll, and the
	// status bar surfaces the percentage. Initial view shows " 0%".
	body := strings.Repeat("line\n", 50)
	m := markdown_overlay.New(
		func(_ string, _ int) string { return body },
		markdown_overlay.WithTitle("T"),
		markdown_overlay.WithSource("x"),
	).SetSize(60, 12)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "0%") {
		t.Errorf("status bar missing initial percent:\n%s", out)
	}
}
