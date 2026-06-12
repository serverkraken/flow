package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ImportTar extracts a tar.gz snapshot into the notebook directory.
type ImportTar struct {
	Rooter ports.NotebookRooter
	Tar    ports.TarSnapshot
}

// NewImportTar wires the use case with its required ports.
func NewImportTar(rooter ports.NotebookRooter, tar ports.TarSnapshot) *ImportTar {
	return &ImportTar{Rooter: rooter, Tar: tar}
}

// ImportTarInput carries the archive path and the conflict-resolution mode.
type ImportTarInput struct {
	Archive string
	Mode    ports.ConflictMode
}

// ImportTarOutput reports the directory the archive was extracted into.
type ImportTarOutput struct {
	Target string
}

// Execute extracts the archive into the notebook root.
func (u *ImportTar) Execute(ctx context.Context, in ImportTarInput) (ImportTarOutput, error) {
	target := u.Rooter.Root()
	if err := u.Tar.Import(ctx, in.Archive, target, in.Mode); err != nil {
		return ImportTarOutput{}, fmt.Errorf("import tar: %w", err)
	}
	return ImportTarOutput{Target: target}, nil
}
