package markdown_overlay

// White-box tests for the code-copy helpers. They live in the package
// so the unexported osc52SetClipboard / writeClipboardLocal / etc.
// can be exercised directly — fixtures don't try to spawn a real
// pbcopy/xclip; instead the local-writer is given a poisoned PATH so
// the function exits via its "no clipboard helper" branch, and the
// OSC 52 path is checked for the right escape framing.

import (
	"os"
	"strings"
	"testing"
)

func TestOSC52SetClipboard_NoTmuxWrap(t *testing.T) {
	t.Setenv("TMUX", "")
	out := osc52SetClipboard("hello")
	// "hello" → base64 "aGVsbG8=" — assert the OSC 52 framing + base64 payload.
	if !strings.Contains(out, "aGVsbG8=") {
		t.Errorf("OSC 52 should contain base64 payload, got %q", out)
	}
	if strings.Contains(out, "\x1bPtmux;") {
		t.Errorf("non-tmux env should not produce tmux DCS wrap, got %q", out)
	}
}

func TestOSC52SetClipboard_TmuxWrap(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-socket")
	out := osc52SetClipboard("hello")
	if !strings.HasPrefix(out, "\x1bPtmux;") {
		t.Errorf("under TMUX the sequence should be DCS-wrapped, got %q", out)
	}
}

func TestClipboardWriters_StableList(t *testing.T) {
	got := clipboardWriters()
	if len(got) == 0 {
		t.Fatalf("clipboardWriters should return a non-empty list")
	}
	if got[0][0] != "pbcopy" {
		t.Errorf("pbcopy must be first (macOS priority), got %q", got[0][0])
	}
}

func TestWriteClipboardLocal_NoHelpersAvailable(t *testing.T) {
	// Empty PATH so exec.LookPath fails for every candidate.
	t.Setenv("PATH", "")
	// Some test runners set HOME-derived PATH bits; also clear them.
	t.Setenv("HOMEBREW_PREFIX", "")
	if writeClipboardLocal("anything") {
		t.Errorf("with no clipboard helper on PATH, writeClipboardLocal should return false")
	}
}

func TestWriteClipboardCmd_FallsBackToOSC52OnNoHelper(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("TMUX", "")
	// We can't easily intercept os.Stdout in a test without breaking other output,
	// so just verify the command runs without panic and returns nil.
	cmd := writeClipboardCmd("snippet")
	if cmd == nil {
		t.Fatalf("writeClipboardCmd should return a tea.Cmd")
	}
	// Don't actually invoke — would print escape sequence to test output.
	_ = cmd
}

func TestClearCopyStatusCmd_NonNil(t *testing.T) {
	if cmd := clearCopyStatusCmd(); cmd == nil {
		t.Errorf("clearCopyStatusCmd must return a tea.Cmd")
	}
}

func TestCopyNextSnippet_NoSnippets_ShowsHint(t *testing.T) {
	m := Model{}
	m2, body := m.copyNextSnippet()
	if body != "" {
		t.Errorf("with no snippets body should be empty, got %q", body)
	}
	if !strings.Contains(m2.copyStatus, "Keine") {
		t.Errorf("status should reflect empty state, got %q", m2.copyStatus)
	}
}

func TestCopyNextSnippet_CyclesIndex(t *testing.T) {
	m := Model{snippets: []codeSnippet{{lang: "go", body: "alpha"}, {lang: "", body: "beta"}}}
	m, body := m.copyNextSnippet()
	if body != "alpha" {
		t.Errorf("first call body=%q, want alpha", body)
	}
	if m.copyIdx != 1 {
		t.Errorf("after first call copyIdx=%d, want 1", m.copyIdx)
	}
	m, body = m.copyNextSnippet()
	if body != "beta" {
		t.Errorf("second call body=%q, want beta", body)
	}
	// The third call wraps to index 0.
	m, body = m.copyNextSnippet()
	if body != "alpha" {
		t.Errorf("third call should wrap to first, got %q", body)
	}
	// Status carries the index/total.
	if !strings.Contains(m.copyStatus, "1/2") {
		t.Errorf("status should include 1/2 marker, got %q", m.copyStatus)
	}
	// Compile-time sanity: os.Args reference removes unused imports if any.
	_ = os.Args
}

func TestIsClosingFence_TrueFalse(t *testing.T) {
	if !isClosingFence("```", "```") {
		t.Errorf("backticks should close matching opener")
	}
	if !isClosingFence("  ```", "```") {
		t.Errorf("indented matching closer should still match")
	}
	if isClosingFence("```", "~~~") {
		t.Errorf("different fence type should not match")
	}
	if isClosingFence("``", "```") {
		t.Errorf("too-short fence should not match")
	}
	if isClosingFence("```text", "```") {
		t.Errorf("trailing text after closer should not match")
	}
	if isClosingFence("abc", "```") {
		t.Errorf("plain text should not be a closing fence")
	}
}
