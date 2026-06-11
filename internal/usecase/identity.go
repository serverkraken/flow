package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// IdentityProvider is the narrow read surface the Identity use case needs.
// Satisfied by *httpapi.Identity.
type IdentityProvider interface {
	Me(ctx context.Context) (domain.User, error)
}

// Identity resolves which user a client runs as via the bearer API.
// After R2a, local-user fallback and first-login adoption are removed;
// the server is the single source of truth.
type Identity struct {
	provider IdentityProvider
}

// NewIdentity constructs the Identity use case.
func NewIdentity(provider IdentityProvider) *Identity {
	return &Identity{provider: provider}
}

// ResolveActiveUser returns the authenticated user from the bearer API.
// Returns ports.ErrTokenNotFound when no valid token is stored.
func (i *Identity) ResolveActiveUser(ctx context.Context) (domain.User, error) {
	u, err := i.provider.Me(ctx)
	if err != nil {
		if errors.Is(err, ports.ErrTokenNotFound) {
			return domain.User{}, ports.ErrTokenNotFound
		}
		return domain.User{}, err
	}
	return u, nil
}
