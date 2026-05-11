// Code-snippet copy support: parse the markdown source for fenced
// code blocks, expose them via a `c` keybinding that cycles through
// them and pushes the next snippet to the system clipboard via the
// OSC 52 escape sequence (which tmux + ghostty + iTerm + WezTerm
// pass through to the OS clipboard, no shell-out / OS dep needed).

package view

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// codeSnippet pairs a fenced code block's body with its info-string
// language so the status bar can show "copied terraform 2/3".
type codeSnippet struct {
	lang string
	body string
}

// fenceLine matches the OPEN fence of a code block — three or more
// backticks (or tildes), optional language. Captures the fence run
// and the language. Reusing the same regex for the close means the
// run length must match.
var fenceLine = regexp.MustCompile("(?m)^(\\s*)(`{3,}|~{3,})([^\\n]*)$")

// extractCodeSnippets walks src for fenced code blocks and returns
// each block's (language, body) pair in document order. Indented
// (4-space) code blocks aren't included — `c` is meant to grab
// fenced snippets, which are the ones with a language hint and the
// natural unit a user wants on their clipboard.
func extractCodeSnippets(src string) []codeSnippet {
	var (
		snips     []codeSnippet
		open      bool
		openFence string
		openLang  string
		body      strings.Builder
	)
	for _, line := range strings.Split(src, "\n") {
		if !open {
			m := fenceLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			open = true
			openFence = m[2]
			openLang = strings.TrimSpace(m[3])
			body.Reset()
			continue
		}
		// Closing fence: same character (` or ~) and at least the
		// opening run length.
		if isClosingFence(line, openFence) {
			snips = append(snips, codeSnippet{
				lang: openLang,
				body: strings.TrimRight(body.String(), "\n"),
			})
			open = false
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	return snips
}

// isClosingFence returns true when line is a closing fence for an
// open block whose opener was openFence (a run of `s or ~s).
func isClosingFence(line, openFence string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, openFence[:1]) {
		return false
	}
	if len(trimmed) < len(openFence) {
		return false
	}
	for i := 0; i < len(openFence); i++ {
		if trimmed[i] != openFence[0] {
			return false
		}
	}
	// Anything after the closing run must be blank.
	return strings.TrimSpace(trimmed[len(openFence):]) == ""
}

// osc52SetClipboard returns the OSC 52 escape sequence that tells
// supporting terminals (Ghostty, iTerm, WezTerm, kitty, foot, …)
// to push content to the system clipboard. When running inside
// tmux, the sequence is wrapped in a DCS tmux passthrough so the
// inner terminal still sees it (tmux's own OSC 52 handling depends
// on `set-clipboard on/external` and is unreliable across versions).
//
// $TMUX is read in-place rather than via Env in main.go: it describes
// the runtime terminal multiplexer, not app config. See the A1
// platform-detection carve-out in cmd/flow/main.go's Env doc.
func osc52SetClipboard(content string) string {
	osc := ansi.SetSystemClipboard(content)
	if os.Getenv("TMUX") == "" {
		return osc
	}
	// tmux DCS passthrough: `\ePtmux;\e<inner>\e\\`. Every literal
	// ESC inside the inner sequence has to be doubled so tmux's
	// parser doesn't terminate the DCS early.
	inner := strings.ReplaceAll(osc, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;\x1b" + inner + "\x1b\\"
}

// writeClipboardCmd returns a tea.Cmd that pushes content into the
// system clipboard via the most reliable channel available:
//
//  1. pbcopy / xclip / wl-copy on the local host (atomic, bypasses
//     terminal & multiplexer entirely) — this is the path that
//     "just works" on macOS and any X11/Wayland Linux box.
//  2. OSC 52 escape sequence as a fallback when no local clipboard
//     binary is on PATH (typically: SSH session into a headless
//     box). The DCS-tmux wrap kicks in automatically when running
//     inside tmux.
//
// Returns nil as the tea.Msg — the side-effect IS the point; the
// model has nothing to react to.
func writeClipboardCmd(content string) tea.Cmd {
	return func() tea.Msg {
		if writeClipboardLocal(content) {
			return nil
		}
		// Fallback: emit OSC 52 to stdout. Tmux wrap is added when
		// $TMUX is set so the inner terminal still sees it.
		_, _ = os.Stdout.WriteString(osc52SetClipboard(content))
		return nil
	}
}

// writeClipboardLocal pipes content into the first clipboard CLI
// it finds on PATH. Returns true on success; false when no helper
// is available (caller falls back to OSC 52).
func writeClipboardLocal(content string) bool {
	for _, cmd := range clipboardWriters() {
		bin, err := exec.LookPath(cmd[0])
		if err != nil {
			continue
		}
		c := exec.Command(bin, cmd[1:]...)
		c.Stdin = strings.NewReader(content)
		if err := c.Run(); err == nil {
			return true
		}
	}
	return false
}

// clipboardWriters returns the prioritised list of (binary, args)
// candidates to try for piping clipboard content into. macOS lists
// pbcopy first; Linux lists wl-copy (Wayland) before xclip (X11)
// because most modern distros default to Wayland. Order matters —
// the first one that exists + succeeds wins.
func clipboardWriters() [][]string {
	return [][]string{
		{"pbcopy"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
}
