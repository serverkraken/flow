package ports

// Tmux abstracts the tmux-binary calls flow makes. Adapters implement via
// `os/exec`; tests use a fake that records the calls.
type Tmux interface {
	// RefreshClient triggers a status-bar redraw (`tmux refresh-client -S`).
	RefreshClient() error
	// ShowOption returns a global tmux option value (e.g. "@tn_green").
	// Empty string when unset or unavailable; never an error — callers
	// fall back to defaults.
	ShowOption(name string) string
	// CurrentSessionName returns the active tmux session name, or "" when
	// flow runs outside tmux.
	CurrentSessionName() string
	// ListSessions returns the names of all running tmux sessions. Used
	// by the projects screen to highlight which projects already have
	// a session attached.
	ListSessions() ([]string, error)
	// HasSession reports whether a session of the given name exists.
	HasSession(name string) bool
	// NewSessionAt creates a detached tmux session with the given name,
	// rooted at dir. No-op if a session of the same name already exists.
	NewSessionAt(name, dir string) error
	// SwitchClient attaches the current client to the named session.
	SwitchClient(name string) error
	// SplitWindowH spawns a horizontal split running `cmd args...`.
	SplitWindowH(cmd string, args ...string) error
	// RunTmuxAction backgrounds a tmux subcommand (the action argument is
	// a tmux subcommand string, e.g. "source-file ~/.tmux.conf" or
	// "display-popup -E 'flow today'"). The adapter wraps it as
	// `tmux run-shell -b "tmux <action>"` so the caller (typically a TUI
	// process) returns immediately while tmux executes in the background.
	// The wrapping is the adapter's responsibility — callers MUST pass
	// the action without a leading "tmux " prefix.
	RunTmuxAction(action string) error
}
