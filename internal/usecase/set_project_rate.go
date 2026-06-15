package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetProjectRate validates and stores (or clears) a project's per-hour rate.
type SetProjectRate struct {
	Projects ports.ProjectStore
}

// Execute validates rate (when non-nil) and delegates to the store.
// A nil rate clears any existing rate. Validation rules:
//   - Amount must be >= 0.
//   - Currency must be exactly 3 characters (ISO-4217).
func (uc SetProjectRate) Execute(ctx context.Context, ownerID, projectID string, rate *domain.Money) error {
	if rate != nil {
		if rate.Amount < 0 {
			return domain.ErrInvalidRate
		}
		if len(rate.Currency) != 3 {
			return domain.ErrInvalidRate
		}
	}
	return uc.Projects.SetRate(ctx, ownerID, projectID, rate)
}
