// Package cheatsheetfs implements ports.CheatsheetReader by reading a
// single Markdown file (typically ~/.tmux/cheatsheet.md). Rendering is
// deliberately decoupled from loading — the same content can be piped
// through different MarkdownRenderer implementations without forcing
// the reader to know about styles.
package cheatsheetfs
