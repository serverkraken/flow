package editor

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/shellsafe"
)

// Runner runs name args... and returns combined stdout. Pulled out as
// a function value so unit tests can stub the shell without launching
// real processes.
type Runner func(name string, args ...string) ([]byte, error)

// PathFunc resolves a note ID to its absolute filesystem path. Returns
// "" when the ID can't be resolved (the launcher then refuses with a
// clear error rather than spawning an editor on a blank path).
type PathFunc func(id string) string

// ArgsFunc returns the argv-style command that opens path in the
// user's editor (i.e. {bin, ...flags, path}). Production wires this
// to the kompendium nvimeditor adapter's $VISUAL/$EDITOR/nvim
// resolver. Naming intentionally avoids the editor.EditorArgsFunc
// stutter — call sites read editor.ArgsFunc.
type ArgsFunc func(path string) ([]string, error)

// Launcher implements ports.NoteLauncher by spawning a tmux split that
// runs the user's editor. Read-only viewing went in-process during the
// glow-migration (Heute `o` opens heuteDialogNoteView via the integrated
// markdown renderer); the external $FLOW_NOTE_VIEWER pipeline is gone.
type Launcher struct {
	run        Runner
	pathOf     PathFunc
	editorArgs ArgsFunc
}

// New constructs a Launcher with the production runner. pathOf must
// return absolute filesystem paths (typically a closure over the
// kompendium NoteStore); editorArgs returns the editor argv
// ($VISUAL/$EDITOR/nvim resolution).
func New(pathOf PathFunc, editorArgs ArgsFunc) *Launcher {
	return &Launcher{
		run:        defaultRunner,
		pathOf:     pathOf,
		editorArgs: editorArgs,
	}
}

// NewWithRunner is for tests.
func NewWithRunner(pathOf PathFunc, editorArgs ArgsFunc, r Runner) *Launcher {
	return &Launcher{run: r, pathOf: pathOf, editorArgs: editorArgs}
}

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Open spawns a tmux split that runs the user's editor on the note's
// path. The path is resolved in-process via pathOf; the editor argv
// (binary + flags) comes from editorArgs.
//
// Each argv token is single-quote-escaped and joined into one shell
// command passed as a single positional to `tmux split-window -h`.
// tmux always runs split-window's trailing args through /bin/sh -c, so
// a path containing a space (or `;`, `$`, backtick) would otherwise
// re-split inside sh — opening a file with the wrong name and creating
// a stray new buffer for the leftover tokens.
func (l *Launcher) Open(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("note id darf nicht leer sein")
	}
	path := l.pathOf(id)
	if path == "" {
		return errors.New("note path nicht auflösbar")
	}
	argv, err := l.editorArgs(path)
	if err != nil {
		return fmt.Errorf("resolve editor: %w", err)
	}
	if len(argv) == 0 {
		return errors.New("editor command empty")
	}
	cmd := joinShellArgv(argv)
	_, err = l.run("tmux", "split-window", "-h", cmd)
	return err
}

// joinShellArgv quotes each argv token via shellsafe.Quote and joins
// with spaces. Empty argv returns "" — callers check for that case
// before calling. The quoting rule itself lives in shellsafe so the
// pager (which composes a bash -c command from a viewer + path pair)
// shares the exact implementation.
func joinShellArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellsafe.Quote(a)
	}
	return strings.Join(parts, " ")
}
