// Package editor implements ports.NoteLauncher by spawning tmux
// horizontal splits.
//
// Open shells the user's editor flow via the kompendium binary
// (`tmux split-window -h <kompendium> open <id>`); kompendium itself
// chooses the editor it runs.
//
// View resolves the note's filesystem path via `kompendium path <id>`
// and then opens it with the configured note viewer (typically glow)
// in a horizontal split. Resolving the path before launching the
// viewer means the viewer doesn't need any kompendium awareness.
package editor
