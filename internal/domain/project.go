package domain

import (
	"fmt"
	"time"
)

// ProjectStatus is the lifecycle state of a Project.
type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectPaused   ProjectStatus = "paused"
	ProjectArchived ProjectStatus = "archived"
)

// Project is the First-Class hub work sessions book against. M1a uses a
// minimal field set; the heavier foundation fields (repos/paths/links/…)
// arrive in later migrations.
type Project struct {
	ID        string        `json:"id"`
	OwnerID   string        `json:"-"`
	Name      string        `json:"name"`
	Slug      string        `json:"slug"`
	Color     string        `json:"color"`
	Glyph     string        `json:"glyph"`
	Description string      `json:"description"`
	UpstreamGit string      `json:"upstreamGit"`
	Rate      *Money        `json:"rate,omitempty"` // optional per-hour rate (nil = unset)
	Status    ProjectStatus `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// NewProject builds a validated, active Project stamped at now.
func NewProject(id, ownerID, name, slug string, now time.Time) (Project, error) {
	switch {
	case id == "":
		return Project{}, fmt.Errorf("%w: id required", ErrInvalidProject)
	case ownerID == "":
		return Project{}, fmt.Errorf("%w: owner required", ErrInvalidProject)
	case name == "":
		return Project{}, fmt.Errorf("%w: name required", ErrInvalidProject)
	case slug == "":
		return Project{}, fmt.Errorf("%w: slug required", ErrInvalidProject)
	}
	return Project{
		ID: id, OwnerID: ownerID, Name: name, Slug: slug,
		Status: ProjectActive, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Validate checks the invariants enforced on every mutation: a project needs a
// name and slug, and its status must be one of the three known states.
func (p Project) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalidProject)
	case p.Slug == "":
		return fmt.Errorf("%w: slug required", ErrInvalidProject)
	case !ValidProjectColor(p.Color):
		return fmt.Errorf("%w: invalid color %q", ErrInvalidProject, p.Color)
	case !ValidProjectGlyph(p.Glyph):
		return fmt.Errorf("%w: invalid glyph %q", ErrInvalidProject, p.Glyph)
	}
	switch p.Status {
	case ProjectActive, ProjectPaused, ProjectArchived:
		return nil
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidProject, p.Status)
	}
}
