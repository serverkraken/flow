package worktime

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestNewBriefView_UsesRendererWhenWired(t *testing.T) {
	t.Parallel()
	mr := &testutil.FakeMarkdownRenderer{Prefix: "[md] "}
	bv := newBriefView("Brief · KW 18 2026", "# Brief\n", 100, 40, Deps{MarkdownRenderer: mr})
	out := ansi.Strip(bv.View())
	if !strings.Contains(out, "[md]") {
		t.Errorf("rendered body should contain renderer prefix [md]; got:\n%s", out)
	}
}

func TestNewBriefView_FallsBackToRawWhenNoRenderer(t *testing.T) {
	t.Parallel()
	bv := newBriefView("title", "# raw body", 100, 40, Deps{})
	out := ansi.Strip(bv.View())
	if !strings.Contains(out, "raw body") {
		t.Errorf("without MarkdownRenderer the raw body should still render; got:\n%s", out)
	}
}

func TestBriefView_CloseKeyEmitsExitMsg(t *testing.T) {
	t.Parallel()
	bv := newBriefView("t", "body", 80, 30, Deps{})
	for _, k := range []string{"q", "esc", "b"} {
		var msg tea.KeyMsg
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		} else {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := bv.Update(msg)
		if cmd == nil {
			t.Fatalf("close-key %q expected non-nil cmd", k)
		}
		if _, ok := cmd().(markdown_overlay.ExitMsg); !ok {
			t.Errorf("close-key %q: expected ExitMsg, got %T", k, cmd())
		}
	}
}
