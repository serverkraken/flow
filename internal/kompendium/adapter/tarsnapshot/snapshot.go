package tarsnapshot

import "github.com/serverkraken/flow/internal/kompendium/ports"

// Snapshot implements ports.TarSnapshot.
type Snapshot struct{}

// New returns an empty Snapshot. The struct holds no state.
func New() Snapshot { return Snapshot{} }

var _ ports.TarSnapshot = Snapshot{}
