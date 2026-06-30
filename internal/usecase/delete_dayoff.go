package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteDayOff removes one manual day-off and emits dayoff.changed.
type DeleteDayOff struct {
	Store   ports.DayOffStore
	Emitter ports.Emitter
}

func (uc DeleteDayOff) Execute(ctx context.Context, ownerID string, day time.Time) error {
	if err := uc.Store.Delete(ctx, ownerID, day); err != nil {
		return err
	}
	uc.Emitter.Emit(ctx, domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
