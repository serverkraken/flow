package worktime

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestNewBriefView_UsesRendererWhenWired(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{Prefix: "[md] "}
	bv := newBriefView("Brief · KW 18 2026", "# Brief\n", 100, 40, Deps{MarkdownRenderer: mr}, theme.Load())
	if bv.rendered == bv.rawBody {
		t.Error("rendered should differ from rawBody when MarkdownRenderer is wired")
	}
	if !bv.ready {
		t.Error("ready must be true after construction")
	}
}

func TestNewBriefView_FallsBackToRawWhenNoRenderer(t *testing.T) {
	t.Parallel()
	bv := newBriefView("title", "# raw", 100, 40, Deps{}, theme.Load())
	if bv.rendered != bv.rawBody {
		t.Errorf("without MarkdownRenderer rendered must == rawBody; got %q vs %q", bv.rendered, bv.rawBody)
	}
}

func TestBriefView_UpdateKey_QuitsOnCloseKey(t *testing.T) {
	t.Parallel()
	bv := newBriefView("t", "body", 80, 30, Deps{}, theme.Load())
	for _, k := range []string{"q", "esc", "b"} {
		_, _, close := bv.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		// "esc" + "b" only register via Type for esc; runes work for the
		// rest. KeyEsc has its own type.
		if k == "esc" {
			_, _, close = bv.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
		}
		if !close {
			t.Errorf("close-key %q should signal close=true", k)
		}
	}
}

func TestBriefView_UpdateKey_ScrollDoesNotClose(t *testing.T) {
	t.Parallel()
	bv := newBriefView("t", "body", 80, 30, Deps{}, theme.Load())
	_, _, close := bv.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	if close {
		t.Error("scroll key must not signal close")
	}
}

func TestBriefView_Resize_UpdatesViewport(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{}
	bv := newBriefView("t", "body", 80, 30, Deps{MarkdownRenderer: mr}, theme.Load())
	resized := bv.resize(120, 50, Deps{MarkdownRenderer: mr})
	if got := resized.vp.Width; got != briefViewWidth(120) {
		t.Errorf("vp.Width after resize = %d, want %d", got, briefViewWidth(120))
	}
	if got := resized.vp.Height; got != briefViewHeight(50) {
		t.Errorf("vp.Height after resize = %d, want %d", got, briefViewHeight(50))
	}
}
