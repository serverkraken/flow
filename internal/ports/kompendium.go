package ports

// NoteLauncher opens a kompendium note in the user's editor (typically
// tmux split + nvim). The K4 in-tree migration removed the previous
// KompendiumGateway port — flow no longer reads notebook metadata via
// a shell-out, the kompendium use cases serve that role directly
// through `flow kompendium <verb>`. View() is gone in the glow-migration:
// read-only rendering happens in-process via the integrated markdown
// renderer (Heute `o`-Key → heuteDialogNoteView), not via an external
// tmux-split tool.
type NoteLauncher interface {
	Open(id string) error
}
