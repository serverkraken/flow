package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoNoteStore persists RepoNotes. Phase 1 / M2 lays the schema; M4
// drives the actual editing + sync.
type RepoNoteStore interface {
	GetByRepo(userID, repoID string) (domain.RepoNote, error)
	Upsert(n domain.RepoNote) error
}

// ErrRepoNoteNotFound is returned by RepoNoteStore when no note exists for the given (user, repo) pair.
var ErrRepoNoteNotFound = errSentinel("flow: repo note not found")
