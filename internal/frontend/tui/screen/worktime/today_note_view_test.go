package worktime

// Tests für den integrierten Note-Viewer im Heute-Screen. Schwerpunkt:
// F1-Resize-Pfad rendert Markdown mit der neuen Breite neu — sonst
// zerlaufen Tabellen / Code-Blöcke nach einem tmux-Pane-Resize. Der
// Resize-Pfad liegt nach dem Component-Lift in markdown_overlay
// (SetSize → rerender → RenderFunc-Aufruf); dieser Test pinned die
// Integration durch heute.Update.

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// fakeNoteReader liefert dem heute-Screen einen Note-Body, ohne dass ein
// Disk-IO-Pfad gebraucht wird. Read kann auch fehlschlagen, um den
// SetError-Pfad in openNoteViewDialog zu testen.
type fakeNoteReader struct {
	body string
	err  error
}

func (r fakeNoteReader) Read(_ string) (string, error) {
	return r.body, r.err
}

// TestHeute_WindowSizeMsg_ReRendersNoteViewBody pins the F1 resize
// fix: when the integrated note view is open, a WindowSizeMsg re-runs
// the MarkdownRenderer at the new inner width. After the Component-
// Lift the path is heute.Update → noteView.SetSize → overlay.rerender
// → RenderFunc closure → deps.MarkdownRenderer.Render. The test
// asserts mr.LastWidth changes after the WindowSizeMsg.
func TestHeute_WindowSizeMsg_ReRendersNoteViewBody(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	nr := fakeNoteReader{body: "# heading\n\n| col |\n| --- |\n| x   |"}
	h := newHeute(theme.Load(), Deps{MarkdownRenderer: mr, NoteReader: nr})
	h.width = 80
	h.height = 30
	h.attachedNotes = []string{"daily/2026-05-11"}

	model, _ := h.openNoteViewDialog()
	h = model.(heute)
	if h.noteView == nil {
		t.Fatal("openNoteViewDialog: noteView is nil after attach")
	}
	openTimeWidth := mr.LastWidth
	if openTimeWidth == 0 {
		t.Fatal("renderer was never called during overlay construction")
	}

	// Simulate a tmux pane resize: terminal goes from 80×30 → 120×40.
	updated, _ := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := updated.(heute)
	if got.noteView == nil {
		t.Fatal("noteView dropped during WindowSizeMsg")
	}
	if mr.LastWidth == openTimeWidth {
		t.Errorf("renderer not re-invoked after resize: LastWidth stayed at %d (open-time width)",
			openTimeWidth)
	}
	if mr.LastWidth < openTimeWidth {
		t.Errorf("renderer received smaller width after enlargement: open=%d, post-resize=%d",
			openTimeWidth, mr.LastWidth)
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

func TestHeute_OpenNoteViewDialog_NoAttached_ReturnsToast(t *testing.T) {
	t.Parallel()
	h := newHeute(theme.Load(), Deps{})
	model, cmd := h.openNoteViewDialog()
	got, ok := model.(heute)
	if !ok {
		t.Fatalf("got %T, want heute", model)
	}
	if got.noteView != nil {
		t.Error("noteView must stay nil when no notes are attached")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd carrying the info-toast message")
	}
	if _, ok := cmd().(heuteActionDoneMsg); !ok {
		t.Errorf("expected heuteActionDoneMsg, got %T", cmd())
	}
}

func TestHeute_OpenNoteViewDialog_ReadError_SetsError(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	nr := fakeNoteReader{err: fmt.Errorf("disk gone")}
	h := newHeute(theme.Load(), Deps{MarkdownRenderer: mr, NoteReader: nr})
	h.width = 80
	h.height = 30
	h.attachedNotes = []string{"daily/2026-05-11"}

	model, _ := h.openNoteViewDialog()
	got := model.(heute)
	if got.noteView == nil {
		t.Fatal("noteView must be non-nil even on read-error (SetError path)")
	}
	if got.dialog != heuteDialogNoteView {
		t.Errorf("dialog = %v, want heuteDialogNoteView", got.dialog)
	}
}
