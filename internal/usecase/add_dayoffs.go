package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrHolidayNotManual rejects attempts to store a holiday — holidays are
// computed from the user's Bundesland, never persisted.
var ErrHolidayNotManual = errors.New("holiday kind is computed, not stored")

// AddDayOffs expands [from,to] into per-day rows (skipping weekends when
// asked), upserts each, and publishes a single dayoff.changed event.
type AddDayOffs struct {
	Store ports.DayOffStore
	Bus   ports.EventBus
}

func (uc AddDayOffs) Execute(ctx context.Context, ownerID string, from, to time.Time, kind domain.Kind, label string, targetPerDay time.Duration, skipWeekends bool) error {
	if kind == domain.KindHoliday {
		return ErrHolidayNotManual
	}
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return domain.ErrInvalidDayOff
	}
	days := domain.ExpandRange(from, to, kind, label, targetPerDay, skipWeekends)
	if len(days) == 0 {
		return domain.ErrInvalidDayOff
	}
	for _, d := range days {
		if err := uc.Store.Add(ctx, ownerID, d); err != nil {
			return err
		}
	}
	uc.Bus.Publish(domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
