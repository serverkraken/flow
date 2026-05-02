package ports

// NoteLauncher opens a kompendium note in the user's environment. Open()
// uses an editor (typically tmux split + nvim); View() is read-only
// (typically tmux split + glow). The K4 in-tree migration removed the
// previous KompendiumGateway port — flow no longer reads notebook
// metadata via a shell-out, the kompendium use cases serve that role
// directly through `flow kompendium <verb>`.
type NoteLauncher interface {
	Open(id string) error
	View(id string) error
}
