package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteDayOff removes one manual day-off and publishes dayoff.changed.
type DeleteDayOff struct {
	Store ports.DayOffStore
	Bus   ports.EventBus
}

func (uc DeleteDayOff) Execute(ctx context.Context, ownerID string, day time.Time) error {
	if err := uc.Store.Delete(ctx, ownerID, day); err != nil {
		return err
	}
	uc.Bus.Publish(domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
