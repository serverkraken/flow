package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

const ctxKeyUser ctxKey = 2

// WithUser attaches the resolved domain.User to a context. Used by handlers
// downstream of NewBearerMiddleware when UserStore is provided.
func WithUser(ctx context.Context, u domain.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}

// UserFromContext returns the domain.User injected by the bearer middleware.
// Returns zero value and false when no user is present (e.g. UserStore nil).
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(domain.User)
	return u, ok
}

// userCache is a process-local cache of OIDC sub → domain.User. Avoids
// hitting the UserStore on every request; the row only changes when the
// user logs in (rare) so the cache TTL is effectively the server lifetime.
type userCache struct {
	m sync.Map // map[string]domain.User keyed by sub
}

func (c *userCache) get(sub string) (domain.User, bool) {
	v, ok := c.m.Load(sub)
	if !ok {
		return domain.User{}, false
	}
	return v.(domain.User), true
}

func (c *userCache) put(sub string, u domain.User) {
	c.m.Store(sub, u)
}

// ensureUser looks up the user for the given sub in the cache; on a miss it
// calls store.EnsureBySub and populates the cache. Returns the user and
// whether the operation succeeded. On store error it writes a 500 and returns
// false — the caller must stop processing.
func ensureUser(
	w http.ResponseWriter,
	_ *http.Request,
	store ports.UserStore,
	cache *userCache,
	sub, email, name string,
) (domain.User, bool) {
	if u, ok := cache.get(sub); ok {
		return u, true
	}
	u, err := store.EnsureBySub(sub, email, name)
	if err != nil {
		slog.Error("bearer: EnsureBySub failed",
			slog.String("sub", sub),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return domain.User{}, false
	}
	cache.put(sub, u)
	return u, true
}
