//go:build !windows

// Package linkstsv implements ports.LinkStore as a TSV file at the
// configured path (typically ~/.tmux/worktime-links.tsv).
//
// File format: one row per attachment, `date<TAB>noteID`. Insertion
// order is preserved so the TUI day view shows links in the order the
// user added them.
//
// Add is idempotent on (date, noteID). Validation of the noteID
// (rejecting empty strings or TSV-breaking characters) is the use
// case's job — see LinkWriter.
package linkstsv
