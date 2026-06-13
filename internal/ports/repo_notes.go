package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoNoteStore persists RepoNotes. Phase 1 / M2 lays the schema; M4
// drives the actual editing + sync (Plan C).
type RepoNoteStore interface {
	GetByRepo(userID, repoID string) (domain.RepoNote, error)
	Upsert(n domain.RepoNote) error
	Delete(userID, id string) error
	PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error)
}

// ErrRepoNoteNotFound is returned by RepoNoteStore when no note exists for
// the given (user, repo) pair.
var ErrRepoNoteNotFound = errSentinel("flow: repo note not found")

// ErrRepoNoteVersionConflict is returned by server-side RepoNoteStore.Upsert
// when the expected version does not match the stored row.
var ErrRepoNoteVersionConflict = errSentinel("flow: repo note version conflict")
