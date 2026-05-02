package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ImportBundle fetches and merges a git-bundle into the notebook.
type ImportBundle struct {
	Store  ports.NoteStore
	Bundle ports.NotebookBundler
}

// NewImportBundle wires the use case with its required ports.
func NewImportBundle(store ports.NoteStore, bundle ports.NotebookBundler) *ImportBundle {
	return &ImportBundle{Store: store, Bundle: bundle}
}

// ImportBundleInput identifies the bundle file to import.
type ImportBundleInput struct {
	BundlePath string
}

// ImportBundleOutput reports the directory the bundle merged into.
type ImportBundleOutput struct {
	Target string
}

// Execute fetches and merges the bundle into the notebook root.
func (u *ImportBundle) Execute(ctx context.Context, in ImportBundleInput) (ImportBundleOutput, error) {
	target := u.Store.Root()
	if err := u.Bundle.ImportBundle(ctx, target, in.BundlePath); err != nil {
		return ImportBundleOutput{}, fmt.Errorf("import bundle: %w", err)
	}
	return ImportBundleOutput{Target: target}, nil
}
