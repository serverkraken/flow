// Package ports defines the application's external boundaries: adapters consumed (TokenStore, MarkdownRenderer) and ports exposed (DayOffStore, Resources).
package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// WorktimeMachine ist die Client-Sicht auf die Server-Statemachine
// (Spec §7): synchrone Writes, Konflikte kommen als Sentinels zurück.
// Reads laufen weiter über ActiveSessionStore.
type WorktimeMachine interface {
	Start(projectID, tag, note string) (domain.ActiveSession, error) // 409 ⇒ ErrActiveSessionConflict
	Stop(projectID string) (domain.Session, error)                   // 404 ⇒ ErrActiveSessionNotFound
	Pause(projectID string) (domain.ActiveSession, error)
	Resume(projectID string) (domain.ActiveSession, error)
	CorrectStart(projectID string, startedAt time.Time) (domain.ActiveSession, error)
}
