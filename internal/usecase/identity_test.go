package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeIdentityProvider is an in-memory IdentityProvider for unit tests.
type fakeIdentityProvider struct {
	user domain.User
	err  error
}

func (f *fakeIdentityProvider) Me(_ context.Context) (domain.User, error) {
	return f.user, f.err
}

// TestIdentityResolveActiveUser_returnsUser verifies the happy path: when the
// provider returns a user, ResolveActiveUser returns the same user.
func TestIdentityResolveActiveUser_returnsUser(t *testing.T) {
	want := domain.User{ID: "u1", OIDCSub: "msoent", Email: "soenne@example.com"}
	id := usecase.NewIdentity(&fakeIdentityProvider{user: want})

	got, err := id.ResolveActiveUser(context.Background())
	if err != nil {
		t.Fatalf("ResolveActiveUser: unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID: got %q, want %q", got.ID, want.ID)
	}
	if got.OIDCSub != want.OIDCSub {
		t.Errorf("OIDCSub: got %q, want %q", got.OIDCSub, want.OIDCSub)
	}
}

// TestIdentityResolveActiveUser_loggedOut verifies that when the provider
// returns ports.ErrTokenNotFound (mapped by httpapi.Identity.Me on 401/ErrLoggedOut),
// ResolveActiveUser surfaces ports.ErrTokenNotFound to the caller.
func TestIdentityResolveActiveUser_loggedOut(t *testing.T) {
	id := usecase.NewIdentity(&fakeIdentityProvider{err: ports.ErrTokenNotFound})

	_, err := id.ResolveActiveUser(context.Background())
	if !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("expected ports.ErrTokenNotFound, got %v", err)
	}
}

// TestIdentityResolveActiveUser_otherError verifies that unexpected errors
// from the provider are passed through.
func TestIdentityResolveActiveUser_otherError(t *testing.T) {
	providerErr := errors.New("network timeout")
	id := usecase.NewIdentity(&fakeIdentityProvider{err: providerErr})

	_, err := id.ResolveActiveUser(context.Background())
	if !errors.Is(err, providerErr) {
		t.Fatalf("expected provider error to bubble, got %v", err)
	}
}
