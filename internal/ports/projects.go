package ports

import "github.com/serverkraken/flow/internal/domain"

// ProjectStore persists Worktime-Projects. NOT to be confused with
// SourceDirScanner (file-system source directory listing).
type ProjectStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
	GetBySlug(userID, slug string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(p domain.Project) error
	TouchLastUsed(userID, id string) error
	Archive(userID, id string) error
}

// ErrProjectNotFound is returned by ProjectStore when the requested project does not exist.
var ErrProjectNotFound = errSentinel("flow: project not found")
