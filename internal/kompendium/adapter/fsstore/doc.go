// Package fsstore implements ports.NoteStore on top of the filesystem.
//
// The notebook layout is ID-as-path: a note with ID "daily/2026-04-25" lives
// at "<root>/daily/2026-04-25.md", and a note with ID
// "projects/github.com/serverkraken/dotfiles/2026-04-25" lives at
// "<root>/projects/github.com/serverkraken/dotfiles/2026-04-25.md". The
// directly-derivable mapping keeps Get/Put trivially correct and the on-disk
// hierarchy human-browsable.
package fsstore
