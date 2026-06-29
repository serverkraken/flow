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
	ErrNodeNotFound    = errors.New("project not found")
	ErrNodeHasChildren = errors.New("node has children")
	// ErrNodeSlugTaken marks a slug collision: another node under the same parent
	// (or another root, for engagements) already uses this slug. Slugs are unique
	// per sibling set, not globally — the same name may repeat across the tree.
	ErrNodeSlugTaken = errors.New("node slug already taken under this parent")

	ErrSessionNotFound   = errors.New("session not found")
	ErrFeedTokenNotFound = errors.New("feed token not found")
	ErrDocumentNotFound  = errors.New("document not found")
	ErrDocumentExists    = errors.New("document already exists")
	ErrBindingNotFound   = errors.New("ports: binding not found")

	// ErrEmbedTransient marks an embed failure as transient/global — the backend is
	// unavailable or misconfigured (connection error, timeout, HTTP 503/429, or a
	// missing model 404) — rather than caused by one document's content. The embed
	// worker tests for it with errors.Is to decide whether to back off a single
	// document or just stop and retry the whole batch next tick.
	ErrEmbedTransient = errors.New("embed backend transient failure")
)

// NodeStore persists projects. All reads are owner-scoped.
type NodeStore interface {
	Create(ctx context.Context, p domain.Node) (domain.Node, error)
	List(ctx context.Context, ownerID string) ([]domain.Node, error)
	Get(ctx context.Context, ownerID, id string) (domain.Node, error)
	// Update overwrites a project's mutable metadata (name, slug, color, glyph,
	// description, upstream_git, status). Rate is NOT touched (see SetRate).
	// Owner-scoped; returns ErrNodeNotFound for a missing or foreign project.
	Update(ctx context.Context, ownerID string, p domain.Node) (domain.Node, error)
	// SetRate sets (rate != nil) or clears (rate == nil) the project's rate.
	SetRate(ctx context.Context, ownerID, id string, rate *domain.Money) error
	// Delete removes a project. Owner-scoped; returns ErrNodeNotFound if absent
	// or foreign. Returns ErrNodeHasChildren if the node still has children
	// (FK RESTRICT on parent_id).
	Delete(ctx context.Context, ownerID, id string) error
	// Children returns the direct children of parentID (nil = roots) for the
	// given owner, ordered by name.
	Children(ctx context.Context, ownerID string, parentID *string) ([]domain.Node, error)
	// Ancestors returns the node itself and all its ancestors, ordered
	// leaf→root (the node first, then its parent, then the grandparent, …).
	Ancestors(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error)
	// Reparent moves a node to a new parent (nil = make it a root). Owner-scoped;
	// returns ErrNodeNotFound for a missing or foreign node.
	Reparent(ctx context.Context, ownerID, id string, parentID *string) (domain.Node, error)
}

// SessionStore persists work sessions. The DB enforces at most one running
// session per owner (partial unique index); Running returns it if present.
type SessionStore interface {
	Create(ctx context.Context, s domain.WorkSession) (domain.WorkSession, error)
	Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error)
	// Get fetches a single session by id. Owner-scoped; returns
	// ErrSessionNotFound for a missing or foreign session.
	Get(ctx context.Context, ownerID, id string) (domain.WorkSession, error)
	Stop(ctx context.Context, ownerID, id string, nodeID *string, stop time.Time) (domain.WorkSession, error)
	List(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error)
	// ListRange returns sessions with since <= Start < until, newest first.
	// Owner-scoped. Used for past-day views and the overlap check.
	ListRange(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error)
	// Update overwrites a session's project/note/start/stop. Tags are persisted
	// separately via TagStore (the taggings junction), not here. Owner-scoped;
	// returns ErrSessionNotFound for a missing or foreign session.
	Update(ctx context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	// Delete removes a session. Owner-scoped; ErrSessionNotFound if absent.
	Delete(ctx context.Context, ownerID, id string) error
	// ListPage returns the owner's sessions newest-first (start_at DESC),
	// limited to `limit` rows starting at `offset`, plus the total owner count
	// (ignoring limit/offset) for pagination math. Owner-scoped.
	ListPage(ctx context.Context, ownerID string, limit, offset int) (items []domain.WorkSession, total int, err error)
	// TagTimes returns the total tracked minutes per tag for the owner, optionally
	// filtered to sessions whose start_at falls in [from, to). Zero value means
	// unbounded on that side. Results are ordered by minutes DESC, tag ASC.
	TagTimes(ctx context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error)
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

// StaleDoc is a document needing (re)embedding plus its prior consecutive
// embed-failure count, so the worker computes backoff without an extra read.
type StaleDoc struct {
	Doc      domain.Document
	Attempts int
}

// DocumentStore persists compendium documents. All reads are owner-scoped.
// Create returns ErrDocumentExists on a (owner, project, path) collision.
type DocumentStore interface {
	Create(ctx context.Context, d domain.Document) (domain.Document, error)
	Get(ctx context.Context, ownerID, id string) (domain.Document, error)
	// List returns the owner's documents newest-first. When tags are given, only
	// documents containing ALL of them are returned (AND semantics).
	// projectID: nil = no filter; ptr to "none" = unassigned; else a project ID.
	List(ctx context.Context, ownerID string, nodeID *string, tags ...string) ([]domain.Document, error)
	// ListPage returns one page of documents newest-first plus the total count
	// matching the owner/project/tag filter, for server-side pagination.
	ListPage(ctx context.Context, ownerID string, nodeID *string, limit, offset int, tags ...string) ([]domain.Document, int, error)
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
	// projectID: nil = no filter; ptr to "none" = unassigned; else a project ID.
	Search(ctx context.Context, ownerID, q string, nodeID *string, tags []string) ([]domain.SearchHit, error)

	// SetPinned sets (pinned=true) or clears (pinned=false) the pinned flag on
	// one document. Owner-scoped; returns ErrDocumentNotFound if absent or foreign.
	SetPinned(ctx context.Context, ownerID, id string, pinned bool) error

	// SetArchived sets (archived=true) or clears (archived=false) the archived
	// flag. Archiving also clears pinned (archived dominates) and stamps
	// archived_at; un-archiving nulls archived_at and leaves pinned untouched.
	SetArchived(ctx context.Context, ownerID, id string, archived bool) error

	// UpsertByPath inserts a new document or — if a row already exists for
	// (owner_id, coalesce(node_id,''), path) — updates title/body/updated_at
	// while preserving the existing pinned flag. Returns the row's id and
	// updated_at regardless of whether the row was inserted or updated.
	UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (id string, updatedAt time.Time, err error)

	// ListForContext returns the owner's documents matching any of the given
	// nodeIDs or — when includeGlobal is true — those with node_id IS NULL,
	// filtered to the given document types. Tags are hydrated. Used by the
	// compose usecase (A5) to gather instruction+memory docs along the
	// ancestor chain plus optional global docs in a single query.
	ListForContext(ctx context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error)

	// StaleDocuments returns up to limit documents needing (re)embedding
	// (chunks_hash out of date), excluding dead-lettered docs and those still
	// within a backoff window, each with its prior consecutive failure count.
	StaleDocuments(ctx context.Context, limit int) ([]StaleDoc, error)

	// ReplaceChunks atomically replaces a document's chunks with the given
	// (content, embedding) pairs (len-equal, may be empty) and stamps chunks_hash
	// so the document is no longer stale.
	ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error

	// SemanticSearch returns the owner's documents whose chunks are nearest to the
	// query vector (cosine), best chunk per document, optionally AND-filtered by
	// tags, each with that chunk's text as Snippet. Ordered nearest-first.
	// projectID: nil = no filter; ptr to "none" = unassigned; else a project ID.
	SemanticSearch(ctx context.Context, ownerID string, query []float32, nodeID *string, tags []string, limit int) ([]domain.SemanticHit, error)

	// RecordEmbedFailure upserts the per-document embed-failure state used for
	// backoff and dead-lettering.
	RecordEmbedFailure(ctx context.Context, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error
	// ClearEmbedFailure removes a document's recorded embed failure (manual
	// retry); a successful ReplaceChunks clears it implicitly.
	ClearEmbedFailure(ctx context.Context, docID, ownerID string) error
	// EmbedStatus returns the owner-scoped embedding status of one document.
	EmbedStatus(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error)
}

// TagStore is the polymorphic tag registry + junction (B2).
type TagStore interface {
	SetTags(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string, raw []string) ([]domain.Tag, error)
	TagsFor(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) ([]domain.Tag, error)
	TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error)
	FilterIDs(ctx context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error)
	ListTags(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error)
	ClearTaggable(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string) error
	MergeTags(ctx context.Context, ownerID, fromSlug, intoSlug string) error
}

// ProjectBindingStore persists project bindings (remote-slug and path-prefix
// rules). All reads are owner-scoped. Upsert replaces an existing row with
// the same kind-key: (owner, remote_slug) for BindingRemote, or
// (owner, machine_id, path) for BindingPath.
type ProjectBindingStore interface {
	Upsert(ctx context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error)
	DeleteRemote(ctx context.Context, ownerID, remoteSlug string) error
	DeletePath(ctx context.Context, ownerID, machineID, path string) error
	List(ctx context.Context, ownerID string) ([]domain.ProjectBinding, error)
	ListByProject(ctx context.Context, ownerID, projectID string) ([]domain.ProjectBinding, error)
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
