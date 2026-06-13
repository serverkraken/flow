// Package ports defines interfaces for ports.
package ports

import "time"

// Document is a server-side markdown document (Spec §6). Path is the
// '/-separated relative location ("projects/serverkraken/flow/ideen.md");
// RepoKey is non-empty only for repo notes ("git:github.com/foo/bar").
type Document struct {
	ID        string
	UserID    string
	Path      string
	Body      string
	RepoKey   string
	Version   int64
	UpdatedAt time.Time
}

// DocumentEntry is the body-less list/search row.
type DocumentEntry struct {
	Path      string
	RepoKey   string
	Version   int64
	UpdatedAt time.Time
	Snippet   string // FTS-Headline bei Suche, sonst leer
}

// DocumentStore persists markdown documents in the server DB (Spec §6/§7).
type DocumentStore interface {
	// Get returns the document at path or ErrDocumentNotFound.
	Get(userID, path string) (Document, error)
	// GetByRepoKey returns the repo-note alias target or ErrDocumentNotFound.
	GetByRepoKey(userID, repoKey string) (Document, error)
	// List returns entries under prefix (both may be empty), optionally
	// FTS-filtered by query, ordered by path. limit <= 0 → default 200.
	List(userID, prefix, query string, limit int) ([]DocumentEntry, error)
	// Put upserts with If-Match semantics: ifMatch 0 = create-only
	// (conflict when the path exists), N = update-only-if-version-is-N.
	Put(userID, path, body, repoKey string, ifMatch int64) (Document, error)
	// Delete removes the document; idempotent.
	Delete(userID, path string) error
}

// ErrDocumentNotFound is returned when no document exists at (user, path/key).
var ErrDocumentNotFound = errSentinel("flow: document not found")

// ErrDocumentVersionConflict is returned by Put on If-Match mismatch.
var ErrDocumentVersionConflict = errSentinel("flow: document version conflict")
