package worktime

// Tests für den integrierten Note-Viewer im Heute-Screen. Schwerpunkt:
// Resize-Pfad rendert Markdown mit der neuen Breite neu — sonst zerlaufen
// Tabellen / Code-Blöcke nach einem tmux-Pane-Resize (parallel zu
// brief_view.resize). Vorher: WindowSizeMsg-Handler passte nur die
// Viewport-Maße an, ohne den Body neu zu rendern.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestRenderNoteViewBody_NoRenderer_ReturnsBodyVerbatim(t *testing.T) {
	t.Parallel()
	if got := renderNoteViewBody("raw body", 100, Deps{}); got != "raw body" {
		t.Errorf("renderNoteViewBody without renderer = %q, want %q", got, "raw body")
	}
}

func TestRenderNoteViewBody_UsesRendererWidth(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{Prefix: "[md] "}
	got := renderNoteViewBody("body", 100, Deps{MarkdownRenderer: mr})
	if got != "[md] body" {
		t.Errorf("renderNoteViewBody = %q, want %q", got, "[md] body")
	}
	if mr.LastWidth != 94 {
		t.Errorf("LastWidth = %d, want 94 (termW - 6)", mr.LastWidth)
	}
}

func TestRenderNoteViewBody_NarrowTerm_AppliesFloor(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	_ = renderNoteViewBody("body", 10, Deps{MarkdownRenderer: mr})
	if mr.LastWidth != 60 {
		t.Errorf("LastWidth on narrow term = %d, want 60 (floor)", mr.LastWidth)
	}
}

func TestRenderNoteViewBody_RendererError_FallsBackToBody(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{Err: errFakeRender}
	got := renderNoteViewBody("raw fallback", 100, Deps{MarkdownRenderer: mr})
	if got != "raw fallback" {
		t.Errorf("on Render error, want raw body; got %q", got)
	}
}

// TestHeute_WindowSizeMsg_ReRendersNoteViewBody pins the resize fix:
// when the integrated note view is open, a WindowSizeMsg re-runs the
// MarkdownRenderer at the new inner-box width. Before the fix only
// the viewport's W/H changed — the rendered content stayed frozen at
// the open-time width, so tables / code blocks wrapped against the
// old line length and leaked outside the new pane.
func TestHeute_WindowSizeMsg_ReRendersNoteViewBody(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	h := newHeute(theme.Load(), Deps{MarkdownRenderer: mr})
	h.dialog = heuteDialogNoteView
	h.noteViewReady = true
	h.noteViewBody = "# heading\n\n| col |\n| --- |\n| x   |"
	h.width = 80
	h.height = 30

	// Simulate a tmux pane resize: terminal goes from 80×30 → 120×40.
	updated, _ := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got, ok := updated.(heute)
	if !ok {
		t.Fatalf("Update returned %T, want heute", updated)
	}
	if got.noteViewVP.Width != noteViewWidth(120) {
		t.Errorf("vp.Width after resize = %d, want %d", got.noteViewVP.Width, noteViewWidth(120))
	}
	if mr.LastWidth != 114 {
		t.Errorf("MarkdownRenderer.LastWidth after resize = %d, want 114 (120 - 6)", mr.LastWidth)
	}
}

func TestHeute_WindowSizeMsg_NoReRender_WhenDialogClosed(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	h := newHeute(theme.Load(), Deps{MarkdownRenderer: mr})
	// dialog stays heuteDialogNone — resize must not call the renderer.
	_, _ = h.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	if mr.LastWidth != 0 {
		t.Errorf("renderer was called on resize with no open note view (LastWidth=%d)", mr.LastWidth)
	}
}

type fakeRenderErr struct{}

func (fakeRenderErr) Error() string { return "fake render failure" }

var errFakeRender = fakeRenderErr{}
