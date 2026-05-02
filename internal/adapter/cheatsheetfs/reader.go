package cheatsheetfs

import "os"

// Reader returns the raw Markdown content of a cheatsheet file.
type Reader struct {
	path string
}

// New constructs a Reader. path is typically ~/.tmux/cheatsheet.md as
// resolved by the composition root.
func New(path string) *Reader {
	return &Reader{path: path}
}

// Load returns the file contents. A missing or unreadable file is
// surfaced as an error so the cheatsheet screen can show a "no cheat
// sheet found" hint instead of an empty view.
func (r *Reader) Load() (string, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
