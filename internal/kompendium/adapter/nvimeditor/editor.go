package nvimeditor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Editor is a ports.Editor backed by a real OS process invocation.
type Editor struct {
	runner runFunc
}

// runFunc abstracts the real exec call so tests can verify the editor
// resolution + arg passing without launching a real binary.
type runFunc func(ctx context.Context, name string, args ...string) error

// New returns an Editor backed by the real exec runner.
func New() Editor {
	return Editor{runner: realRun}
}

// Edit implements ports.Editor.
//
// The editor command is taken from $VISUAL (POSIX preferred) → $EDITOR →
// "nvim" as last resort. The variable's value is shell-split so common
// constructs like `code -w` or `nvim -O` work; the file path is appended
// as the final argument. stdio is passed through so the editor takes
// over the terminal and the user gets full TUI interaction.
func (e Editor) Edit(ctx context.Context, path string) error {
	bin, args, err := resolveEditor()
	if err != nil {
		return err
	}
	args = append(args, path)
	if err := e.runner(ctx, bin, args...); err != nil {
		return fmt.Errorf("editor %q on %q: %w", bin, path, err)
	}
	return nil
}

// Cmd returns an unstarted *exec.Cmd that opens path in the resolved
// editor with stdio inherited. The Bubble Tea browse view passes the
// result to tea.ExecProcess so the editor can take over the sidekick
// pane without the kompendium binary having to know about tmux.
//
// On a malformed env value (e.g. an unbalanced quote), the fallback
// editor is used — Cmd has no error channel, so degrading gracefully
// beats failing late inside tea.ExecProcess.
func (Editor) Cmd(path string) *exec.Cmd {
	bin, args, err := resolveEditor()
	if err != nil {
		bin, args = defaultEditor, nil
	}
	args = append(args, path)
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func realRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var _ ports.Editor = Editor{}
