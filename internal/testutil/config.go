package testutil

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.ConfigReader = (*FakeConfigReader)(nil)

// FakeConfigReader returns a fixed Config snapshot. Set Cfg directly in
// the test, or use the zero value for the empty-config case.
type FakeConfigReader struct {
	Cfg domain.Config
	Err error // returned by Load when non-nil
}

// Load returns Cfg (and Err if set) — no I/O.
func (f *FakeConfigReader) Load() (domain.Config, error) {
	if f.Err != nil {
		return domain.Config{}, f.Err
	}
	return f.Cfg, nil
}
