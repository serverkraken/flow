package testutil

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.ProjectStore = (*FakeProjectStore)(nil)

// FakeProjectStore is an in-memory ports.ProjectStore for use in worktime tests
// that exercise the new project_picker / ActiveSessions path.
type FakeProjectStore struct {
	Projects []domain.Project
	Err      error
	// TouchedIDs records TouchLastUsed calls for assertions.
	TouchedIDs []string
}

// ListActive returns all non-archived projects for the given user.
func (f *FakeProjectStore) ListActive(userID string) ([]domain.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var out []domain.Project
	for _, p := range f.Projects {
		if p.UserID == userID && p.ArchivedAt == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// ListAll returns all projects (including archived) for the given user.
func (f *FakeProjectStore) ListAll(userID string) ([]domain.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var out []domain.Project
	for _, p := range f.Projects {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

// GetByID finds a project by its ID.
func (f *FakeProjectStore) GetByID(userID, id string) (domain.Project, error) {
	if f.Err != nil {
		return domain.Project{}, f.Err
	}
	for _, p := range f.Projects {
		if p.UserID == userID && p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

// GetBySlug finds a project by its slug.
func (f *FakeProjectStore) GetBySlug(userID, slug string) (domain.Project, error) {
	if f.Err != nil {
		return domain.Project{}, f.Err
	}
	for _, p := range f.Projects {
		if p.UserID == userID && p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

// EnsureBySlug creates a project with the given name+slug and returns it.
// Uses a deterministic fake ID derived from the slug for test assertions.
func (f *FakeProjectStore) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	if f.Err != nil {
		return domain.Project{}, f.Err
	}
	p := domain.Project{
		ID:        "fake-" + slug,
		UserID:    userID,
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now(),
	}
	f.Projects = append(f.Projects, p)
	return p, nil
}

// Upsert updates or inserts a project.
func (f *FakeProjectStore) Upsert(p domain.Project) error {
	if f.Err != nil {
		return f.Err
	}
	for i := range f.Projects {
		if f.Projects[i].ID == p.ID {
			f.Projects[i] = p
			return nil
		}
	}
	f.Projects = append(f.Projects, p)
	return nil
}

// TouchLastUsed records a touch call for assertion in tests.
func (f *FakeProjectStore) TouchLastUsed(_ string, id string) error {
	if f.Err != nil {
		return f.Err
	}
	f.TouchedIDs = append(f.TouchedIDs, id)
	return nil
}

// Archive soft-deletes a project.
func (f *FakeProjectStore) Archive(userID, id string) error {
	if f.Err != nil {
		return f.Err
	}
	now := time.Now()
	for i := range f.Projects {
		if f.Projects[i].UserID == userID && f.Projects[i].ID == id {
			f.Projects[i].ArchivedAt = &now
			return nil
		}
	}
	return ports.ErrProjectNotFound
}
