// Package editor implements ports.NoteLauncher by spawning tmux
// horizontal splits.
//
// Open resolves the note's filesystem path in-process (via the kompendium
// notebook store) and launches the user's editor in a new tmux pane:
// `tmux split-window -h <editor> <path>`. The editor argv is constructed
// by the kompendium nvimeditor adapter so $VISUAL / $EDITOR / nvim
// fallback all behave the same way the standalone kompendium binary did.
//
// Read-only viewing went in-process during the glow-migration: the
// Heute `o`-Key opens heuteDialogNoteView (integrated MarkdownRenderer +
// viewport), so this adapter no longer owns a View() method.
package editor
