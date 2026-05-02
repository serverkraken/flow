package testutil

import (
	"context"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeNotebookBundle records ExportBundle / ImportBundle calls without
// touching git, so use-case tests can drive the bundle code paths.
type FakeNotebookBundle struct {
	mu sync.Mutex

	Exports []BundleExportCall
	Imports []BundleImportCall

	ExportErr error
	ImportErr error
}

// BundleExportCall is one recorded ExportBundle invocation.
type BundleExportCall struct {
	Root string
	Out  string
}

// BundleImportCall is one recorded ImportBundle invocation.
type BundleImportCall struct {
	Root   string
	Bundle string
}

// ExportBundle implements ports.NotebookBundler.
func (f *FakeNotebookBundle) ExportBundle(_ context.Context, root, out string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ExportErr != nil {
		return f.ExportErr
	}
	f.Exports = append(f.Exports, BundleExportCall{Root: root, Out: out})
	return nil
}

// ImportBundle implements ports.NotebookBundler.
func (f *FakeNotebookBundle) ImportBundle(_ context.Context, root, bundle string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ImportErr != nil {
		return f.ImportErr
	}
	f.Imports = append(f.Imports, BundleImportCall{Root: root, Bundle: bundle})
	return nil
}

var _ ports.NotebookBundler = (*FakeNotebookBundle)(nil)
