package testutil

import "github.com/serverkraken/flow/internal/ports"

var _ ports.Tmux = (*FakeTmux)(nil)

// FakeTmux records every method call so tests can assert behaviour.
// Methods return zero values / nil unless the corresponding *Err field
// is set.
type FakeTmux struct {
	Refreshes int
	// Options maps option name → returned value. Unset names return "".
	Options  map[string]string
	Session  string
	Sessions []string
	// Splits collects every SplitWindowH invocation as a flat string for
	// easy comparison: "<cmd> <arg1> <arg2> …".
	Splits []string
	// Shells records every RunShell command for assertion.
	Shells []string
	// New tracks every NewSessionAt invocation as "<name>@<dir>".
	New []string
	// Switches tracks every SwitchClient target.
	Switches []string

	RefreshErr      error
	SplitErr        error
	ShellErr        error
	ListSessionsErr error
	NewSessionErr   error
	SwitchErr       error
}

func (f *FakeTmux) RefreshClient() error {
	f.Refreshes++
	return f.RefreshErr
}

func (f *FakeTmux) ShowOption(name string) string {
	if f.Options == nil {
		return ""
	}
	return f.Options[name]
}

func (f *FakeTmux) CurrentSessionName() string { return f.Session }

func (f *FakeTmux) ListSessions() ([]string, error) {
	if f.ListSessionsErr != nil {
		return nil, f.ListSessionsErr
	}
	out := make([]string, len(f.Sessions))
	copy(out, f.Sessions)
	return out, nil
}

func (f *FakeTmux) HasSession(name string) bool {
	for _, s := range f.Sessions {
		if s == name {
			return true
		}
	}
	return false
}

func (f *FakeTmux) NewSessionAt(name, dir string) error {
	if f.NewSessionErr != nil {
		return f.NewSessionErr
	}
	f.New = append(f.New, name+"@"+dir)
	f.Sessions = append(f.Sessions, name)
	return nil
}

func (f *FakeTmux) SwitchClient(name string) error {
	if f.SwitchErr != nil {
		return f.SwitchErr
	}
	f.Switches = append(f.Switches, name)
	return nil
}

func (f *FakeTmux) SplitWindowH(cmd string, args ...string) error {
	parts := append([]string{cmd}, args...)
	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += " "
		}
		joined += p
	}
	f.Splits = append(f.Splits, joined)
	return f.SplitErr
}

func (f *FakeTmux) RunShell(cmd string) error {
	f.Shells = append(f.Shells, cmd)
	return f.ShellErr
}
