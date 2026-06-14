package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetSettings returns the user's settings plus their active feed tokens, for
// the settings screen.
type GetSettings struct {
	Settings ports.UserSettingsStore
	Tokens   ports.FeedTokenStore
}

func (uc GetSettings) Execute(ctx context.Context, ownerID string) (domain.Settings, []domain.FeedToken, error) {
	set, err := uc.Settings.Get(ctx, ownerID)
	if err != nil {
		return domain.Settings{}, nil, err
	}
	toks, err := uc.Tokens.ListByUser(ctx, ownerID)
	if err != nil {
		return domain.Settings{}, nil, err
	}
	return set, toks, nil
}

// SetBundesland validates and stores the user's Bundesland.
type SetBundesland struct {
	Settings ports.UserSettingsStore
}

func (uc SetBundesland) Execute(ctx context.Context, ownerID, land string) error {
	norm, ok := domain.ValidBundesland(land)
	if !ok {
		return domain.ErrInvalidDayOff
	}
	return uc.Settings.SetBundesland(ctx, ownerID, norm)
}
