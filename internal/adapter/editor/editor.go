// Package editor opens the user's $EDITOR on a temp file (implements ports.Editor).
package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
)

// compile-time assertion: Editor must satisfy ports.Editor.
var _ ports.Editor = Editor{}

// Editor implements ports.Editor by shelling out to $EDITOR (falling back to vi).
type Editor struct{}

// New returns a ready-to-use Editor.
func New() Editor { return Editor{} }

// editorBinary returns the $EDITOR command split into program + leading args
// (falling back to vi).
func editorBinary() (prog string, leading []string) {
	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	return parts[0], parts[1:]
}

// seedTemp writes initial to a fresh temp .md file and returns its name.
func seedTemp(initial []byte) (name string, err error) {
	f, err := os.CreateTemp("", "flow-doc-*.md")
	if err != nil {
		return "", fmt.Errorf("editor: temp file: %w", err)
	}
	name = f.Name()
	if _, err := f.Write(initial); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("editor: write: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("editor: close: %w", err)
	}
	return name, nil
}

// Command seeds a temp .md file with initial and builds an *exec.Cmd that opens
// $EDITOR (fallback vi) on it. readback returns the edited bytes; cleanup removes
// the temp file. The caller wires cmd's stdio (Bubbletea's ExecProcess does this)
// and runs it, then calls readback, then cleanup. cmd is nil on error.
func (Editor) Command(initial []byte) (cmd *exec.Cmd, readback func() ([]byte, error), cleanup func(), err error) {
	name, err := seedTemp(initial)
	if err != nil {
		return nil, nil, nil, err
	}
	prog, leading := editorBinary()
	args := append(leading, name) //nolint:gocritic // intentional: leading may be empty
	cmd = exec.Command(prog, args...)
	readback = func() ([]byte, error) {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("editor: read back: %w", err)
		}
		return data, nil
	}
	cleanup = func() { _ = os.Remove(name) }
	return cmd, readback, cleanup, nil
}

// Edit writes initial to a temp .md file, opens $EDITOR on it inheriting the
// terminal, then reads and returns the result. It is BLOCKING and owns the
// terminal — TUI callers must go through Command + tea.ExecProcess instead.
func (Editor) Edit(ctx context.Context, initial []byte) ([]byte, error) {
	name, err := seedTemp(initial)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(name) }()

	prog, leading := editorBinary()
	args := append(leading, name) //nolint:gocritic // intentional: leading may be empty
	cmd := exec.CommandContext(ctx, prog, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor: run: %w", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("editor: read back: %w", err)
	}
	return data, nil
}
