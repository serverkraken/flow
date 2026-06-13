// Package usecase holds application services. They depend only on ports.
package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var ErrNotAllowed = errors.New("user not in allowlist")

// EnsureUser maps a verified Identity to a stored User, creating it on first
// login. Allow gates access (Phase-1 static allowlist).
type EnsureUser struct {
	Users ports.UserStore
	IDs   ports.IDGen
	Allow func(ports.Identity) bool
}

func (uc EnsureUser) Execute(ctx context.Context, id ports.Identity) (domain.User, error) {
	if uc.Allow == nil || !uc.Allow(id) {
		return domain.User{}, ErrNotAllowed
	}
	switch u, err := uc.Users.GetBySub(ctx, id.Subject); {
	case err == nil:
		return u, nil
	case !errors.Is(err, ports.ErrUserNotFound):
		return domain.User{}, err
	}
	nu, err := domain.NewUser(uc.IDs.NewID(), id.Subject, id.Username, id.Email, id.Name)
	if err != nil {
		return domain.User{}, err
	}
	return uc.Users.UpsertBySub(ctx, nu)
}
