package output

import (
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
)

// Runner runs name with args and returns combined stdout+stderr.
// Production wires defaultRunner; tests pass a recording fake.
type Runner func(name string, args ...string) ([]byte, error)

// StdinRunner runs name with args and pipes stdin to the process'
// stdin. Used by Copy (pbcopy reads the clipboard payload from stdin).
type StdinRunner func(name string, args []string, stdin string) ([]byte, error)

// Targets is the production adapter for ports.Output. It bundles the
// three output sinks (Copy / Pager / SaveFile) so they share one
// constructor and one runner-injection seam for tests.
type Targets struct {
	home  string
	tmux  ports.Tmux
	run   Runner
	runIn StdinRunner
}

// New constructs Targets rooted at home with tmux as the split-window
// dispatcher. home is the absolute path to $HOME (typically resolved
// at the composition root via os.UserHomeDir); SaveFile resolves
// <home>/Downloads/.
func New(home string, tmux ports.Tmux) *Targets {
	return &Targets{
		home:  home,
		tmux:  tmux,
		run:   defaultRunner,
		runIn: defaultStdinRunner,
	}
}

// NewWithRunners is the test constructor. Both runners are injected so
// the test can record calls without spawning real binaries.
func NewWithRunners(home string, tmux ports.Tmux, run Runner, runIn StdinRunner) *Targets {
	if run == nil {
		run = defaultRunner
	}
	if runIn == nil {
		runIn = defaultStdinRunner
	}
	return &Targets{home: home, tmux: tmux, run: run, runIn: runIn}
}

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func defaultStdinRunner(name string, args []string, stdin string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}
