package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ExportTar writes the notebook as a tar.gz at the requested out path.
type ExportTar struct {
	Store ports.NoteStore
	Tar   ports.TarSnapshot
}

// NewExportTar wires the use case with its required ports.
func NewExportTar(store ports.NoteStore, tar ports.TarSnapshot) *ExportTar {
	return &ExportTar{Store: store, Tar: tar}
}

// ExportTarInput configures one Execute call.
type ExportTarInput struct {
	OutPath string
}

// ExportTarOutput reports what was archived and where it landed.
type ExportTarOutput struct {
	Source  string
	OutPath string
}

// Execute resolves the notebook root from the store and asks the snapshot
// adapter to archive it.
func (u *ExportTar) Execute(ctx context.Context, in ExportTarInput) (ExportTarOutput, error) {
	src := u.Store.Root()
	if err := u.Tar.Export(ctx, src, in.OutPath); err != nil {
		return ExportTarOutput{}, fmt.Errorf("export tar: %w", err)
	}
	return ExportTarOutput{Source: src, OutPath: in.OutPath}, nil
}
