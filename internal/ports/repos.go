package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoStore persists Repos. Phase 1 / M2 lays the schema; M4 fills it
// in via the MCP server.
type RepoStore interface {
	EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error)
	GetByID(userID, id string) (domain.Repo, error)
	Upsert(r domain.Repo) error
}
