package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetTargetConfig validates and stores the daily target config (default +
// per-weekday overrides). All minute values must be non-negative.
type SetTargetConfig struct {
	Settings ports.UserSettingsStore
}

func (uc SetTargetConfig) Execute(ctx context.Context, ownerID string, defaultMin int, weekday map[time.Weekday]int) error {
	if defaultMin < 0 {
		return domain.ErrInvalidTarget
	}
	for _, v := range weekday {
		if v < 0 {
			return domain.ErrInvalidTarget
		}
	}
	return uc.Settings.SetTargetConfig(ctx, ownerID, defaultMin, weekday)
}
