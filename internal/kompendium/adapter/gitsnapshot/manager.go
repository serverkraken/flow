package gitsnapshot

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Manager implements ports.NotebookInitializer, ports.NotebookBundler,
// and ports.NotebookRemote against the system git binary.
type Manager struct {
	run runFunc
}

type runFunc func(ctx context.Context, dir string, args ...string) (string, error)

// New returns a Manager backed by the real git binary.
func New() Manager {
	return Manager{run: realRun}
}

// currentBranch returns the symbolic branch name of HEAD. A detached HEAD
// returns "HEAD", which is then used verbatim — almost certainly leading
// to a "no such ref" merge error, which is the right outcome since
// merging into a detached HEAD has no symbolic target anyway.
func (m Manager) currentBranch(ctx context.Context, root string) (string, error) {
	out, err := m.run(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// identityArgs prepends `-c user.name=… -c user.email=…` to args ONLY
// when the host has no git identity configured. With an existing
// identity (e.g. set globally by the user), the real author is
// preserved — important for cross-machine git log readability after
// kompendium snapshot/sync.
func identityArgs(ctx context.Context, run runFunc, root string, args []string) []string {
	if hasIdentity(ctx, run, root) {
		return args
	}
	return append([]string{
		"-c", "user.name=" + fallbackIdentityName,
		"-c", "user.email=" + fallbackIdentityEmail,
	}, args...)
}

// hasIdentity reports whether `git config --get` resolves both user.name
// and user.email in the given root. Either being missing returns false.
func hasIdentity(ctx context.Context, run runFunc, root string) bool {
	name, err := run(ctx, root, "config", "--get", "user.name")
	if err != nil || strings.TrimSpace(name) == "" {
		return false
	}
	email, err := run(ctx, root, "config", "--get", "user.email")
	if err != nil || strings.TrimSpace(email) == "" {
		return false
	}
	return true
}

func realRun(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isExitErr(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

var (
	_ ports.NotebookInitializer = Manager{}
	_ ports.NotebookBundler     = Manager{}
	_ ports.NotebookRemote      = Manager{}
)
