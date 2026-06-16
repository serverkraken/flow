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
	ErrDocumentNotFound  = errors.New("document not found")
	ErrDocumentExists    = errors.New("document already exists")
)

// ProjectStore persists projects. All reads are owner-scoped.
type ProjectStore interface {
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	List(ctx context.Context, ownerID string) ([]domain.Project, error)
	Get(ctx context.Context, ownerID, id string) (domain.Project, error)
	// SetRate sets (rate != nil) or clears (rate == nil) the project's rate.
	SetRate(ctx context.Context, ownerID, id string, rate *domain.Money) error
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
// default row (Bundesland "NW", DefaultTargetMin 480, no weekday overrides)
// for users that never saved settings. SetTargetConfig replaces the daily
// target config wholesale (default + the full override set).
type UserSettingsStore interface {
	Get(ctx context.Context, userID string) (domain.Settings, error)
	SetBundesland(ctx context.Context, userID, land string) error
	SetTargetConfig(ctx context.Context, userID string, defaultMin int, weekday map[time.Weekday]int) error
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

// DocumentStore persists compendium documents. All reads are owner-scoped.
// Create returns ErrDocumentExists on a (owner, project, path) collision.
type DocumentStore interface {
	Create(ctx context.Context, d domain.Document) (domain.Document, error)
	Get(ctx context.Context, ownerID, id string) (domain.Document, error)
	// List returns the owner's documents newest-first. When tags are given, only
	// documents containing ALL of them are returned (AND semantics).
	List(ctx context.Context, ownerID string, tags ...string) ([]domain.Document, error)
	Update(ctx context.Context, d domain.Document) (domain.Document, error)
	Delete(ctx context.Context, ownerID, id string) error
	// ReplaceLinks rewrites the outbound wikilink targets of one document
	// (delete-then-insert). Empty targets clears them.
	ReplaceLinks(ctx context.Context, srcDocID, ownerID string, targets []string) error
	// Backlinks returns the owner's documents whose recorded outbound links
	// include targetPath (candidate sources; the use case re-resolves scope).
	Backlinks(ctx context.Context, ownerID, targetPath string) ([]domain.Document, error)
	// Search returns owner documents matching q (FTS + fuzzy), ranked, each with
	// a highlighted snippet. When tags are given, results are AND-filtered to
	// documents carrying all of them. Empty q is not expected here (callers use
	// List for the no-query path).
	Search(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error)

	// StaleDocuments returns up to limit documents whose chunks are missing or
	// out of date (chunks_hash != md5(title||body)), across all owners, for the
	// embedding worker.
	StaleDocuments(ctx context.Context, limit int) ([]domain.Document, error)

	// ReplaceChunks atomically replaces a document's chunks with the given
	// (content, embedding) pairs (len-equal, may be empty) and stamps chunks_hash
	// so the document is no longer stale.
	ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error

	// SemanticSearch returns the owner's documents whose chunks are nearest to the
	// query vector (cosine), best chunk per document, optionally AND-filtered by
	// tags, each with that chunk's text as Snippet. Ordered nearest-first.
	SemanticSearch(ctx context.Context, ownerID string, query []float32, tags []string, limit int) ([]domain.SemanticHit, error)
}

// Editor opens an interactive editor on initial content and returns the
// edited bytes. Used by the TUI for document bodies.
type Editor interface {
	Edit(ctx context.Context, initial []byte) ([]byte, error)
}

// Embedder turns texts into embedding vectors (one per input, order-preserving).
// Implementations are batched. A non-nil error means the backend (e.g. Ollama) is
// unavailable; callers degrade gracefully.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// DocChangeNotifier is notified after a document is created or updated, so the
// embedding worker can re-embed promptly. Optional for callers (nil → no-op).
type DocChangeNotifier interface {
	DocumentChanged()
}
