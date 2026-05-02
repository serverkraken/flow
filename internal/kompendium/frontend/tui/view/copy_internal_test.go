package view

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const noteWithSnippets = "intro\n\n" +
	"```go\nfunc Hello() {}\n```\n\n" +
	"more prose\n\n" +
	"```bash\necho hello\n```\n"

// TestCopy_CKeyReturnsCmd: pressing `c` returns a non-nil Cmd
// whose execution side-effects the clipboard. The actual stdout-
// writing path is exercised by TestCopy_WriteCmdEmitsOSC52 below.
func TestCopy_CKeyReturnsCmd(t *testing.T) {
	t.Parallel()
	m := New("note", noteWithSnippets, nil, nil, nil).SetSize(120, 30)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("`c` press should return a Cmd")
	}
}

// TestCopy_OSC52Helper exercises the OSC 52 builder directly so we
// can verify the DCS-tmux wrap kicks in via $TMUX without depending
// on a local pbcopy/xclip in the test environment. NOT parallel —
// uses t.Setenv which the std lib forbids inside t.Parallel().
func TestCopy_OSC52Helper(t *testing.T) {
	t.Setenv("TMUX", "")
	plain := osc52SetClipboard("hello")
	if !strings.Contains(plain, "\x1b]52;c;") {
		t.Errorf("OSC 52 prefix missing: %q", plain)
	}

	t.Setenv("TMUX", "fake-session")
	wrapped := osc52SetClipboard("hello")
	if !strings.HasPrefix(wrapped, "\x1bPtmux;") {
		t.Errorf("tmux DCS wrap missing for $TMUX env: %q", wrapped)
	}
	if !strings.HasSuffix(wrapped, "\x1b\\") {
		t.Errorf("tmux DCS terminator missing: %q", wrapped)
	}
}

// TestCopy_CKeyCyclesThroughSnippets: a second `c` press jumps to
// the next snippet — surfaces in the status bar (`copied … 2/2`).
func TestCopy_CKeyCyclesThroughSnippets(t *testing.T) {
	t.Parallel()
	m := New("note", noteWithSnippets, nil, nil, nil).SetSize(120, 30)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(model.View(), "1/2") {
		t.Errorf("first copy should show 1/2 in status bar:\n%s", model.View())
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(model.View(), "2/2") {
		t.Errorf("second copy should show 2/2 in status bar:\n%s", model.View())
	}
}

// TestCopy_CKeyOnDocWithoutSnippets: status bar surfaces the "no
// code blocks to copy" message when the doc has none.
func TestCopy_CKeyOnDocWithoutSnippets(t *testing.T) {
	t.Parallel()
	m := New("note", "just prose, no code\n", nil, nil, nil).SetSize(120, 30)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(model.View(), "no code blocks") {
		t.Errorf("status bar should explain why nothing was copied:\n%s", model.View())
	}
}

// TestCopy_FooterAdvertisesCKeyOnlyWhenSnippetsExist: the footer
// hint shows `c copy code` only when there's something to copy.
func TestCopy_FooterAdvertisesCKeyOnlyWhenSnippetsExist(t *testing.T) {
	t.Parallel()
	withSnips := New("a", noteWithSnippets, nil, nil, nil).SetSize(120, 30).View()
	if !strings.Contains(withSnips, "copy code") {
		t.Errorf("footer should advertise `c copy code` when snippets exist:\n%s", withSnips)
	}
	withoutSnips := New("b", "just prose\n", nil, nil, nil).SetSize(120, 30).View()
	if strings.Contains(withoutSnips, "copy code") {
		t.Errorf("footer should hide `c copy code` when no snippets:\n%s", withoutSnips)
	}
}

// TestCopy_StatusClearsOnTickMsg: the clearCopyStatusMsg drives
// the status bar back to empty after the 2-second delay tick.
func TestCopy_StatusClearsOnTickMsg(t *testing.T) {
	t.Parallel()
	m := New("note", noteWithSnippets, nil, nil, nil).SetSize(120, 30)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(model.View(), "copied") {
		t.Fatalf("baseline: status bar should show copied message")
	}
	cleared, _ := model.Update(clearCopyStatusMsg{})
	if strings.Contains(cleared.View(), "copied") {
		t.Errorf("status bar should be cleared after clearCopyStatusMsg")
	}
}

// TestCopy_ClipboardWritersListed: the writer-priority list always
// includes pbcopy first so macOS finds the working helper without
// scanning a long PATH.
func TestCopy_ClipboardWritersListed(t *testing.T) {
	t.Parallel()
	got := clipboardWriters()
	if len(got) == 0 {
		t.Fatal("clipboardWriters returned empty list")
	}
	if got[0][0] != "pbcopy" {
		t.Errorf("pbcopy should be first candidate; got %v", got[0])
	}
}

// TestCopy_WriteClipboardCmdDoesNotPanic: the cmd body runs to
// completion regardless of whether a clipboard helper is on PATH —
// when nothing's available the OSC 52 fallback writes to stdout,
// which is harmless in tests.
func TestCopy_WriteClipboardCmdDoesNotPanic(t *testing.T) {
	t.Parallel()
	cmd := writeClipboardCmd("hello")
	if cmd == nil {
		t.Fatal("writeClipboardCmd should return non-nil cmd")
	}
	msg := cmd()
	if msg != nil {
		t.Errorf("writeClipboardCmd msg should be nil (side-effect only); got %T", msg)
	}
}

// TestExtractCodeSnippets_ParsesFencedBlocks covers the parser
// directly: backtick fences, tilde fences, and the language
// info-string pickup.
func TestExtractCodeSnippets_ParsesFencedBlocks(t *testing.T) {
	t.Parallel()
	src := "```go\nbody1\nline2\n```\n\n" +
		"~~~bash\ncmd\n~~~\n\n" +
		"```\nno-lang\n```\n"
	snips := extractCodeSnippets(src)
	if len(snips) != 3 {
		t.Fatalf("want 3 snippets, got %d", len(snips))
	}
	if snips[0].lang != "go" || snips[0].body != "body1\nline2" {
		t.Errorf("snippet 0 wrong: %+v", snips[0])
	}
	if snips[1].lang != "bash" || snips[1].body != "cmd" {
		t.Errorf("snippet 1 wrong: %+v", snips[1])
	}
	if snips[2].lang != "" || snips[2].body != "no-lang" {
		t.Errorf("snippet 2 wrong: %+v", snips[2])
	}
}
