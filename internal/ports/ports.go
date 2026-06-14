// Package ports declares the interfaces the application core depends on.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

var ErrUserNotFound = errors.New("user not found")

type Clock interface{ Now() time.Time }

type IDGen interface{ NewID() string }

type UserStore interface {
	UpsertBySub(ctx context.Context, u domain.User) (domain.User, error)
	GetBySub(ctx context.Context, sub string) (domain.User, error)
	GetByID(ctx context.Context, id string) (domain.User, error)
}

// Identity is the verified result of a bearer token.
type Identity struct {
	Subject  string
	Username string
	Email    string
	Name     string
	Groups   []string
}

// Token is a stored OAuth token set for the CLI/TUI client.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

// TokenStore persists the CLI/TUI token between invocations.
type TokenStore interface {
	Save(t Token) error
	Load() (t Token, ok bool, err error)
	Clear() error
}

type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Identity, error)
}

// EventBus is in-process pub/sub for live events. Subscribe returns a channel
// and a cancel func that unsubscribes and closes the channel.
type EventBus interface {
	Publish(ev domain.Event)
	Subscribe(userID string) (events <-chan domain.Event, cancel func())
}

var (
	ErrProjectNotFound   = errors.New("project not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrFeedTokenNotFound = errors.New("feed token not found")
)

// ProjectStore persists projects. All reads are owner-scoped.
type ProjectStore interface {
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	List(ctx context.Context, ownerID string) ([]domain.Project, error)
	Get(ctx context.Context, ownerID, id string) (domain.Project, error)
}

// SessionStore persists work sessions. The DB enforces at most one running
// session per owner (partial unique index); Running returns it if present.
type SessionStore interface {
	Create(ctx context.Context, s domain.WorkSession) (domain.WorkSession, error)
	Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error)
	Stop(ctx context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error)
	List(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error)
}

// DayOffStore persists manual day-offs (vacation/sick). Holidays are computed,
// never stored. All reads are owner-scoped. Add upserts on (owner, day).
type DayOffStore interface {
	Add(ctx context.Context, ownerID string, d domain.DayOff) error
	Delete(ctx context.Context, ownerID string, day time.Time) error
	ListRange(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error)
}

// UserSettingsStore persists per-user preferences. Get lazily returns a
// default row (Bundesland "NW") for users that never saved settings.
type UserSettingsStore interface {
	Get(ctx context.Context, userID string) (domain.Settings, error)
	SetBundesland(ctx context.Context, userID, land string) error
}

// FeedTokenStore mints and resolves calendar-feed tokens. Resolve only
// returns owners for active (non-revoked) tokens. Create stores a token the
// caller already minted. Revoke is idempotent.
type FeedTokenStore interface {
	Create(ctx context.Context, ft domain.FeedToken) error
	Resolve(ctx context.Context, token string) (ownerID string, err error)
	ListByUser(ctx context.Context, userID string) ([]domain.FeedToken, error)
	Revoke(ctx context.Context, userID, token string) error
}
