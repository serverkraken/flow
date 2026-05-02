package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ExportBundle writes a git-bundle of the notebook to the requested path.
type ExportBundle struct {
	Store  ports.NoteStore
	Bundle ports.NotebookBundler
}

// NewExportBundle wires the use case with its required ports.
func NewExportBundle(store ports.NoteStore, bundle ports.NotebookBundler) *ExportBundle {
	return &ExportBundle{Store: store, Bundle: bundle}
}

// ExportBundleInput configures one Execute call.
type ExportBundleInput struct {
	OutPath string
}

// ExportBundleOutput reports what was bundled and where it landed.
type ExportBundleOutput struct {
	Source  string
	OutPath string
}

// Execute resolves the notebook root and asks the bundler to write the
// archive.
func (u *ExportBundle) Execute(ctx context.Context, in ExportBundleInput) (ExportBundleOutput, error) {
	src := u.Store.Root()
	if err := u.Bundle.ExportBundle(ctx, src, in.OutPath); err != nil {
		return ExportBundleOutput{}, fmt.Errorf("export bundle: %w", err)
	}
	return ExportBundleOutput{Source: src, OutPath: in.OutPath}, nil
}
