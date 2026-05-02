// Package editor implements ports.NoteLauncher by spawning tmux
// horizontal splits.
//
// Open resolves the note's filesystem path in-process (via the kompendium
// notebook store) and launches the user's editor in a new tmux pane:
// `tmux split-window -h <editor> <path>`. The editor argv is constructed
// by the kompendium nvimeditor adapter so $VISUAL / $EDITOR / nvim
// fallback all behave the same way the standalone kompendium binary did.
//
// View resolves the same path and opens it with the configured note
// viewer (typically glow) in a horizontal split. Both flows previously
// shelled out to the kompendium binary; K4.E folded that into the
// in-tree adapters.
package editor
