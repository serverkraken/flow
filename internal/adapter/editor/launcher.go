package editor

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs name args... and returns combined stdout. Pulled out as
// a function value so unit tests can stub the shell without launching
// real processes.
type Runner func(name string, args ...string) ([]byte, error)

// Launcher implements ports.NoteLauncher by spawning tmux splits.
type Launcher struct {
	run           Runner
	kompendiumBin string
	noteViewer    string
}

// New constructs a Launcher with the production runner. kompendiumBin
// is the executable name (resolved by composition root from
// $KOMPENDIUM_BIN); noteViewer is the read-only viewer for View
// (typically "glow", overridable via $FLOW_NOTE_VIEWER at the
// composition-root level).
func New(kompendiumBin, noteViewer string) *Launcher {
	return &Launcher{
		run:           defaultRunner,
		kompendiumBin: kompendiumBin,
		noteViewer:    noteViewer,
	}
}

// NewWithRunner is for tests.
func NewWithRunner(kompendiumBin, noteViewer string, r Runner) *Launcher {
	return &Launcher{run: r, kompendiumBin: kompendiumBin, noteViewer: noteViewer}
}

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Open spawns a tmux split that runs `<kompendiumBin> open <id>`,
// letting kompendium choose the editor.
func (l *Launcher) Open(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("note id darf nicht leer sein")
	}
	_, err := l.run("tmux", "split-window", "-h", l.kompendiumBin, "open", id)
	return err
}

// View resolves the note's path via kompendium and then opens it with
// the configured note viewer.
func (l *Launcher) View(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("note id darf nicht leer sein")
	}
	out, err := l.run(l.kompendiumBin, "path", id)
	if err != nil {
		return fmt.Errorf("kompendium path: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return errors.New("note path nicht auflösbar")
	}
	_, err = l.run("tmux", "split-window", "-h", l.noteViewer, path)
	return err
}
