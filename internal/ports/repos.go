package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoStore persists Repos. Phase 1 / M2 lays the schema; M4 fills it in
// via the RepoNotes use case (CLI today, MCP server in Plan D).
type RepoStore interface {
	EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error)
	GetByID(userID, id string) (domain.Repo, error)
	Upsert(r domain.Repo) error
	// PullSince returns rows with Version > since for userID, ordered by
	// Version ASC. hasMore is true when the page filled to limit and more
	// rows likely exist beyond it.
	PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error)
}

// ErrRepoNotFound is returned by RepoStore.GetByID when no row exists.
var ErrRepoNotFound = errSentinel("flow: repo not found")

// ErrRepoVersionConflict is returned by server-side RepoStore.Upsert when
// the expected version does not match the stored row.
var ErrRepoVersionConflict = errSentinel("flow: repo version conflict")
