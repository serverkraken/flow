package tmuxbridge

import (
	"os/exec"
	"strings"
)

// Runner runs the named program with args and returns combined stdout
// (stderr is dropped). Errors include the exit-code error from os/exec.
type Runner func(name string, args ...string) ([]byte, error)

// Bridge wraps tmux CLI calls behind ports.Tmux.
type Bridge struct {
	run Runner
}

// New constructs a Bridge that uses os/exec for the runner. The tmux
// binary is resolved via PATH on each call.
func New() *Bridge {
	return &Bridge{run: defaultRunner}
}

// NewWithRunner is for tests. Pass a Runner that records calls and
// returns canned responses.
func NewWithRunner(r Runner) *Bridge {
	return &Bridge{run: r}
}

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// RefreshClient triggers a status-bar redraw.
func (b *Bridge) RefreshClient() error {
	_, err := b.run("tmux", "refresh-client", "-S")
	return err
}

// ShowOption returns the value of a global tmux option (the leading "@"
// is added automatically). Empty string when unset or when tmux fails;
// callers fall back to defaults instead of branching on errors.
func (b *Bridge) ShowOption(name string) string {
	out, err := b.run("tmux", "show-options", "-gqv", "@"+name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CurrentSessionName returns the current tmux session name, or "" when
// flow runs outside tmux.
func (b *Bridge) CurrentSessionName() string {
	out, err := b.run("tmux", "display-message", "-p", "#{session_name}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ListSessions returns the names of all running tmux sessions, in tmux's
// natural order. tmux's "no server running" error surfaces as a non-nil
// error so the caller can distinguish "outside tmux" from "no sessions".
func (b *Bridge) ListSessions() ([]string, error) {
	out, err := b.run("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	body := strings.TrimRight(string(out), "\n")
	if body == "" {
		return nil, nil
	}
	var names []string
	for _, ln := range strings.Split(body, "\n") {
		if name := strings.TrimSpace(ln); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// HasSession reports whether a session of the given name exists. The
// implementation goes through ListSessions so a missing tmux server
// returns false (no sessions at all) without conflating that with
// "session absent" — the older `tmux has-session -t name` exit-code 1
// covered both, masking the no-server case.
func (b *Bridge) HasSession(name string) bool {
	names, err := b.ListSessions()
	if err != nil {
		return false
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// NewSessionAt creates a detached session at dir. No-op when a session
// of the same name already exists.
func (b *Bridge) NewSessionAt(name, dir string) error {
	if b.HasSession(name) {
		return nil
	}
	_, err := b.run("tmux", "new-session", "-d", "-s", name, "-c", dir)
	return err
}

// SwitchClient attaches the active client to the named session.
func (b *Bridge) SwitchClient(name string) error {
	_, err := b.run("tmux", "switch-client", "-t", name)
	return err
}

// SplitWindowH spawns a horizontal split running cmd args... .
func (b *Bridge) SplitWindowH(cmd string, args ...string) error {
	a := append([]string{"split-window", "-h", cmd}, args...)
	_, err := b.run("tmux", a...)
	return err
}

// RunShell schedules a shell command via `tmux run-shell -b` so the
// caller can continue immediately while tmux executes the action in the
// background. Used by the palette dispatcher.
func (b *Bridge) RunShell(cmd string) error {
	_, err := b.run("tmux", "run-shell", "-b", cmd)
	return err
}
