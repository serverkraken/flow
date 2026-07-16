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

const MaxDayOffRangeDays = 366

var ErrDayOffRangeTooLarge = errors.New("day-off range exceeds 366 days")

// ValidateDayOffRange bounds inclusive calendar-day ranges without expanding
// them first. Add and HTTP list requests share the same per-request ceiling.
func ValidateDayOffRange(from, to time.Time) error {
	if to.Before(from) {
		return domain.ErrInvalidDayOff
	}
	days := 0
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		days++
		if days > MaxDayOffRangeDays {
			return ErrDayOffRangeTooLarge
		}
	}
	return nil
}

// AddDayOffs expands [from,to] into per-day rows (skipping weekends when
// asked), upserts each, and emits a single dayoff.changed event.
type AddDayOffs struct {
	Store   ports.DayOffStore
	Emitter ports.Emitter
}

func (uc AddDayOffs) Execute(ctx context.Context, ownerID string, from, to time.Time, kind domain.Kind, label string, targetPerDay time.Duration, skipWeekends bool) error {
	if kind == domain.KindHoliday {
		return ErrHolidayNotManual
	}
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return domain.ErrInvalidDayOff
	}
	if err := ValidateDayOffRange(from, to); err != nil {
		return err
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
	uc.Emitter.Emit(ctx, domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
