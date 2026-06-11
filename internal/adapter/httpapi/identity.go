package httpapi

// Identity fetches the logged-in user identity from GET /api/v1/me-bearer.
// The result is cached; the cache is cleared on ErrLoggedOut so the next
// call re-fetches after a new login.

import (
	"context"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

// Identity caches the logged-in user's identity.
type Identity struct {
	c     *Client
	cache resourceCache[domain.User]
}

// NewIdentity constructs an Identity adapter backed by c.
func NewIdentity(c *Client) *Identity { return &Identity{c: c} }

// Me returns the cached logged-in user, fetching from the server when needed.
// On ErrLoggedOut the cache is cleared and the error is returned as-is.
func (id *Identity) Me(ctx context.Context) (domain.User, error) {
	if cached, ok := id.cache.get(); ok {
		return cached, nil
	}
	var raw struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	err := id.c.doJSON(ctx, http.MethodGet, "/api/v1/me-bearer", nil, -1, &raw)
	if err != nil {
		if errors.Is(err, ErrLoggedOut) {
			id.cache.invalidate()
		}
		return domain.User{}, err
	}
	u := domain.User{
		OIDCSub:     raw.Sub,
		Email:       raw.Email,
		DisplayName: raw.Name,
		// ID is empty — the server assigns the DB UUID; this is identity-only
	}
	id.cache.put(u)
	return u, nil
}
