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

func TestUpdate_CloseKeyEmitsExitMsg(t *testing.T) {
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource("x"),
	).SetSize(40, 10)
	for _, key := range []string{"q", "esc", "b"} {
		var msg tea.KeyMsg
		switch key {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("key %q: expected non-nil cmd from close-key", key)
		}
		if _, ok := cmd().(markdown_overlay.ExitMsg); !ok {
			t.Errorf("key %q: expected ExitMsg, got %T", key, cmd())
		}
	}
}

func TestUpdate_NonCloseKeyDoesNotExit(t *testing.T) {
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource("body"),
	).SetSize(40, 10)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd != nil {
		if _, ok := cmd().(markdown_overlay.ExitMsg); ok {
			t.Error("non-close key emitted ExitMsg")
		}
	}
}

func TestWithCloseKeys_OverridesDefault(t *testing.T) {
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource("x"),
		markdown_overlay.WithCloseKeys("x"),
	).SetSize(40, 10)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		if _, ok := cmd().(markdown_overlay.ExitMsg); ok {
			t.Error("q exited despite custom CloseKeys=[x]")
		}
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("x expected to emit ExitMsg with custom CloseKeys")
	}
	if _, ok := cmd().(markdown_overlay.ExitMsg); !ok {
		t.Errorf("got %T, want ExitMsg", cmd())
	}
}

func TestSearch_DisabledByDefault(t *testing.T) {
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource("x"),
	).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.CurrentMode() != markdown_overlay.ModeNormal {
		t.Errorf("/ activated search without WithSearch(); mode=%v", m.CurrentMode())
	}
}

func TestSearch_EnabledFindsMatches(t *testing.T) {
	render := func(_ string, _ int) string {
		return "alpha foo bar\nbeta foo qux\ngamma"
	}
	m := markdown_overlay.New(render,
		markdown_overlay.WithTitle("S"),
		markdown_overlay.WithSource("ignored"),
		markdown_overlay.WithSearch(),
	).SetSize(60, 12)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.CurrentMode() != markdown_overlay.ModeSearch {
		t.Fatalf("expected ModeSearch after /, got %v", m.CurrentMode())
	}
	for _, r := range "foo" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.Query(); got != "foo" {
		t.Errorf("query: got %q, want %q", got, "foo")
	}
	if got := m.Matches(); len(got) != 2 {
		t.Errorf("matches: got %v, want 2 (lines 0 + 1 contain foo)", got)
	}
	if m.CurrentMode() != markdown_overlay.ModeNormal {
		t.Errorf("expected ModeNormal after Enter, got %v", m.CurrentMode())
	}
}

func TestSearch_EscCancelsWithoutApplying(t *testing.T) {
	m := markdown_overlay.New(
		func(_ string, _ int) string { return "foo\nbar" },
		markdown_overlay.WithSource("x"),
		markdown_overlay.WithSearch(),
	).SetSize(60, 12)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "foo" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.Query(); got != "" {
		t.Errorf("Esc applied query %q; expected empty", got)
	}
	if m.CurrentMode() != markdown_overlay.ModeNormal {
		t.Errorf("expected ModeNormal after Esc, got %v", m.CurrentMode())
	}
}

func TestSearch_CycleMatchesWithNandShiftN(t *testing.T) {
	m := markdown_overlay.New(
		func(_ string, _ int) string { return "a foo\nb foo\nc foo\nd qux" },
		markdown_overlay.WithSource("x"),
		markdown_overlay.WithSearch(),
	).SetSize(60, 12)
	// open / type foo / enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "foo" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.MatchIndex(); got != 0 {
		t.Errorf("initial MatchIndex: got %d, want 0", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if got := m.MatchIndex(); got != 1 {
		t.Errorf("after n: got %d, want 1", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if got := m.MatchIndex(); got != 0 {
		t.Errorf("after N: got %d, want 0", got)
	}
}

func TestCodeCopy_DisabledByDefault(t *testing.T) {
	body := "```sh\necho hi\n```\n"
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource(body),
	).SetSize(40, 10)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.CopyStatus() != "" {
		t.Errorf("c without WithCodeCopy should not set status; got %q", m.CopyStatus())
	}
}

func TestCodeCopy_EnabledCyclesSnippets(t *testing.T) {
	body := "intro\n```sh\necho one\n```\nmid\n```py\nprint(2)\n```\nend"
	m := markdown_overlay.New(
		func(s string, _ int) string { return s },
		markdown_overlay.WithSource(body),
		markdown_overlay.WithCodeCopy(),
	).SetSize(60, 12)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.CopyStatus(), "1/2") {
		t.Errorf("first c: got status %q, want contains 1/2", m.CopyStatus())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.CopyStatus(), "2/2") {
		t.Errorf("second c: got status %q, want contains 2/2", m.CopyStatus())
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
