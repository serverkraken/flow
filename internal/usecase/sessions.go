package usecase

import (
	"errors"
	"path/filepath"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrSessionNotFound is returned by Sessions.Edit when the target session
// does not exist in the store. Distinct from domain.ErrSessionNotFound
// (which is the index-based sentinel on SessionWriter) so callers can
// match specifically on the ID-based path.
var ErrSessionNotFound = errors.New("flow: session not found")

// Sessions orchestrates ID-based session mutations and smart project
// resolution. It is the Phase-1 M2 replacement for the index-based
// SessionWriter edit/delete paths; the legacy SessionWriter remains
// unchanged until Task 19.
//
// Deliberately does NOT hold LegacyActiveStore or Lock — those belong to
// SessionWriter (active-session lifecycle). ActiveSessions (Task 12) will
// own the start/stop path with the new project-aware semantics.
type Sessions struct {
	users      ports.UserStore
	projects   ports.ProjectStore
	sessions   ports.SessionStore
	sourceDirs ports.SourceDirScanner
}

// NewSessions constructs a Sessions use case.
// users is stored for symmetry with other use cases; Task 12 callers
// will use it. sourceDirs may be nil — step 2 of ResolveProject is
// skipped when nil.
func NewSessions(
	users ports.UserStore,
	projects ports.ProjectStore,
	sessions ports.SessionStore,
	sourceDirs ports.SourceDirScanner,
) *Sessions {
	return &Sessions{
		users:      users,
		projects:   projects,
		sessions:   sessions,
		sourceDirs: sourceDirs,
	}
}

// ResolveProject implements the smart-default cascade from the spec:
//
//  1. explicitID != "" → try GetByID first; if not found try GetBySlug
//     (allows callers to pass either a UUID or a human-readable slug).
//  2. pwd != "" → derive slug from filepath.Base(pwd), call GetBySlug.
//     SourceDirScanner is not consulted here (basename→slug is sufficient
//     for Task 11; Task 12 can add the scanner confirmation if desired).
//  3. ListActive[0] (MRU first).
//  4. EnsureBySlug(userID, "Allgemein", "allgemein").
func (s *Sessions) ResolveProject(userID, explicitID, pwd string) (domain.Project, error) {
	// Step 1: explicit override — try ID first, then slug as fallback.
	// This lets CLI callers pass either a UUID ("abc-123") or a slug
	// ("smoke-project") in --project without requiring them to know which
	// form the system expects.
	if explicitID != "" {
		p, err := s.projects.GetByID(userID, explicitID)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ports.ErrProjectNotFound) {
			return domain.Project{}, err
		}
		// Not found as ID — try as slug.
		return s.projects.GetBySlug(userID, explicitID)
	}

	// Step 2: PWD-based slug lookup.
	if pwd != "" {
		slug := SlugFromName(filepath.Base(pwd))
		p, err := s.projects.GetBySlug(userID, slug)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ports.ErrProjectNotFound) {
			return domain.Project{}, err
		}
		// Not found — continue to step 3.
	}

	// Step 3: MRU (ListActive returns projects MRU-sorted; take first).
	active, err := s.projects.ListActive(userID)
	if err != nil {
		return domain.Project{}, err
	}
	if len(active) > 0 {
		return active[0], nil
	}

	// Step 4: guarantee "Allgemein" exists.
	return s.projects.EnsureBySlug(userID, "Allgemein", "allgemein")
}

// Edit loads the session identified by id, applies mutate to a copy,
// bumps Version, and calls Upsert. Returns ErrSessionNotFound when no
// session with that id exists for userID.
//
// Note: the SessionStore interface does not expose a GetByID method; we
// use LoadFiltered to avoid loading unrelated sessions. A dedicated
// GetByID port method would be cleaner and could be added in Task 19.
func (s *Sessions) Edit(userID, id string, mutate func(*domain.Session)) error {
	rows, err := s.sessions.LoadFiltered(userID, func(row domain.Session) bool {
		return row.ID == id
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrSessionNotFound
	}
	updated := rows[0]
	mutate(&updated)
	updated.Version++
	return s.sessions.Upsert(updated)
}

// Delete removes the session identified by id. Delegates to
// SessionStore.Delete; returns the store error verbatim (including
// ErrSessionVersionConflict if the adapter enforces OCC on delete).
func (s *Sessions) Delete(userID, id string) error {
	return s.sessions.Delete(userID, id)
}
