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

// Edit writes initial to a temp .md file, opens $EDITOR on it inheriting the
// terminal, then reads and returns the result.
func (Editor) Edit(ctx context.Context, initial []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "flow-doc-*.md")
	if err != nil {
		return nil, fmt.Errorf("editor: temp file: %w", err)
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()

	if _, err := f.Write(initial); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("editor: write: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("editor: close: %w", err)
	}

	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	args := append(parts[1:], name) //nolint:gocritic // intentional: parts[1:] may be empty
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor: run %q: %w", ed, err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("editor: read back: %w", err)
	}
	return data, nil
}
