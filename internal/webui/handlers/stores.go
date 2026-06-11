// Package handlers implements the WebUI HTTP handlers.
//
// Narrow store interfaces for the WebUI handler Deps. Both server store
// adapters (sqliteserver until R1's swap, pgstore after) satisfy them
// structurally — the handlers must not know which one is wired.
package handlers

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// SessionsStore is the session surface the WebUI write/read handlers use.
type SessionsStore interface {
	ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error)
	GetByID(userID, id string) (domain.Session, error)
	Upsert(in domain.Session, expectedVersion int64) (domain.Session, error)
	Delete(userID, id string, expectedVersion int64) error
}

// ActiveStore is the active-session lifecycle surface.
type ActiveStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error)
	Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
}

// PauseResumeStore is the R1 pause statemachine surface. Separate from
// ActiveStore because sqliteserver never implements it — the field is
// only wired once pgstore is in (Task 18); handlers nil-guard it.
type PauseResumeStore interface {
	Pause(userID, projectID string) (domain.ActiveSession, error)
	Resume(userID, projectID string) (domain.ActiveSession, error)
}

// ProjectsStore is the project surface.
type ProjectsStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
	GetBySlug(userID, slug string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)
	TouchLastUsed(userID, id string) error
	Archive(userID, id string) error
}
