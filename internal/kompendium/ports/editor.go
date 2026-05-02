package ports

import "context"

// Editor opens a file path for the user to edit interactively. The
// implementation is responsible for choosing the editor (defaults to $EDITOR
// or nvim) and waiting for it to exit.
type Editor interface {
	Edit(ctx context.Context, path string) error
}
