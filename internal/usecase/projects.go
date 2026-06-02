package usecase

import (
	"errors"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects orchestrates Worktime-Project operations: list (driving the
// TUI picker), create (incl. inline-create from the picker), rename,
// archive. Calls TouchLastUsed when a Session starts (called via
// usecase.ActiveSessions.Start, Task 12).
type Projects struct {
	users    ports.UserStore
	projects ports.ProjectStore
}

// NewProjects constructs a Projects use case. users is stored for use by
// Tasks 11-12; Task 10 itself does not call any UserStore method.
func NewProjects(users ports.UserStore, projects ports.ProjectStore) *Projects {
	return &Projects{users: users, projects: projects}
}

// ListActive returns active Projects MRU-first, used by the TUI picker.
func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	return p.projects.ListActive(userID)
}

// Create creates a new Project with auto-generated slug.
//
// Slug rules: lowercase ASCII, spaces → "-", non-[a-z0-9-] stripped,
// collapsed dashes. If the slug collides with an existing one for this
// User, suffix "-2", "-3", ... until unique.
func (p *Projects) Create(userID, name string) (domain.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Project{}, errors.New("project name required")
	}
	base := SlugFromName(name)
	slug := base
	i := 2
	for {
		_, err := p.projects.GetBySlug(userID, slug)
		if errors.Is(err, ports.ErrProjectNotFound) {
			break
		}
		if err != nil {
			return domain.Project{}, err
		}
		slug = base + "-" + strconv.Itoa(i)
		i++
	}
	return p.projects.EnsureBySlug(userID, name, slug)
}

// Rename changes the human-readable name only — slug stays stable.
func (p *Projects) Rename(userID, id, newName string) error {
	pr, err := p.projects.GetByID(userID, id)
	if err != nil {
		return err
	}
	pr.Name = strings.TrimSpace(newName)
	pr.Version++ // local optimistic bump; server may overwrite
	return p.projects.Upsert(pr)
}

// Archive soft-deletes a Project.
func (p *Projects) Archive(userID, id string) error {
	return p.projects.Archive(userID, id)
}

// MarkUsedNow updates LastUsedAt — called from active_sessions.Start.
func (p *Projects) MarkUsedNow(userID, id string) error {
	return p.projects.TouchLastUsed(userID, id)
}

// SlugFromName is the canonical slug-generation. Exposed so the picker
// can preview "the slug we'd assign" for inline-create.
//
// Rules: lowercase ASCII only; spaces, hyphens and underscores become
// a single "-"; all other characters are stripped; leading/trailing
// dashes removed. Returns "unnamed" for inputs that reduce to empty.
func SlugFromName(name string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && sb.Len() > 0 {
				sb.WriteRune('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(sb.String(), "-")
	if s == "" {
		s = "unnamed"
	}
	return s
}
