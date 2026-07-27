package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TagTimeReport aggregates tracked minutes per tag for an owner.
type TagTimeReport struct{ Sessions ports.SessionStore }

func (uc TagTimeReport) Execute(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error) {
	return uc.Sessions.TagTimes(ctx, ownerID, from, to)
}
