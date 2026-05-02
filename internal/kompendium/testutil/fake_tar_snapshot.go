package testutil

import (
	"context"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeTarSnapshot records Export/Import calls without doing any real
// archiving. Use-case tests assert on the recorded calls.
type FakeTarSnapshot struct {
	mu sync.Mutex

	Exports []TarExportCall
	Imports []TarImportCall

	ExportErr error
	ImportErr error
}

// TarExportCall is one recorded Export invocation.
type TarExportCall struct {
	Source string
	Out    string
}

// TarImportCall is one recorded Import invocation.
type TarImportCall struct {
	Archive string
	Target  string
	Mode    ports.ConflictMode
}

// Export implements ports.TarSnapshot.
func (f *FakeTarSnapshot) Export(_ context.Context, source, out string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ExportErr != nil {
		return f.ExportErr
	}
	f.Exports = append(f.Exports, TarExportCall{Source: source, Out: out})
	return nil
}

// Import implements ports.TarSnapshot.
func (f *FakeTarSnapshot) Import(_ context.Context, archive, target string, mode ports.ConflictMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ImportErr != nil {
		return f.ImportErr
	}
	f.Imports = append(f.Imports, TarImportCall{Archive: archive, Target: target, Mode: mode})
	return nil
}

var _ ports.TarSnapshot = (*FakeTarSnapshot)(nil)
