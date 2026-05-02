// Package domain holds the pure value types of kompendium. It must not import
// any I/O package — see CLAUDE.md section 2.1 for the dependency rule.
package domain

import (
	"errors"
	"path"
	"strings"
)

// ErrInvalidID is returned when a string cannot be parsed into a valid note ID.
var ErrInvalidID = errors.New("invalid note id")

// ID identifies a note by its notebook-relative path without the .md suffix
// (e.g. "daily/2026-04-25", "projects/serverkraken/dotfiles/2026-04-25",
// "notes/setup"). IDs are stable across machines because they are
// notebook-relative.
type ID string

// ParseID parses s into an ID. It accepts an optional trailing ".md" and
// rejects empty input, absolute paths, and paths that escape the notebook root.
func ParseID(s string) (ID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrInvalidID
	}
	s = strings.TrimSuffix(s, ".md")
	if s == "" {
		return "", ErrInvalidID
	}
	if strings.HasPrefix(s, "/") {
		return "", ErrInvalidID
	}
	cleaned := path.Clean(s)
	if cleaned != s {
		return "", ErrInvalidID
	}
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalidID
	}
	return ID(cleaned), nil
}

// Path returns the notebook-relative path with the .md suffix.
func (id ID) Path() string {
	return string(id) + ".md"
}

// String returns the ID as a plain string.
func (id ID) String() string {
	return string(id)
}
