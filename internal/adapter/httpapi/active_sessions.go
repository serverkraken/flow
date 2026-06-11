package httpapi

// ActiveSessions implements ports.ActiveSessionStore against the bearer API.
//
// Server scopes all reads to the authenticated user — the userID parameter on
// each method is accepted for interface compliance but ignored.
//
// Upsert and Delete are intentionally unsupported: callers must use
// WorktimeMachine (start/stop/pause/resume) instead of writing active sessions
// directly.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions implements ports.ActiveSessionStore via the server bearer API.
type ActiveSessions struct {
	c     *Client
	cache resourceCache[[]activeDTO]
}

// NewActiveSessions constructs an ActiveSessions adapter backed by c.
func NewActiveSessions(c *Client) *ActiveSessions {
	a := &ActiveSessions{c: c}
	if snap, ok := loadSnapshot(); ok {
		a.cache.put(snap.Active)
	}
	return a
}

// Invalidate forces a cache miss on the next read. Called by the SSE events
// client in main.go's invalidate dispatch when a "worktime" changed event
// arrives from the server.
func (a *ActiveSessions) Invalidate() { a.cache.invalidate() }

var _ ports.ActiveSessionStore = (*ActiveSessions)(nil)

// ListByUser returns all in-progress sessions for the authenticated user.
// userID is ignored — the server scopes the response to the bearer token.
func (a *ActiveSessions) ListByUser(_ string) ([]domain.ActiveSession, error) {
	if cached, ok := a.cache.get(); ok {
		return a.toDomain(cached, ""), nil
	}
	var env itemsEnvelope[activeDTO]
	err := a.c.doJSON(context.Background(), http.MethodGet,
		"/api/v1/worktime/active",
		nil, -1, &env)
	if err != nil {
		if fb, ok := a.cache.fallback(); ok {
			return a.toDomain(fb, ""), nil
		}
		return nil, err
	}
	a.cache.put(env.Items)
	go func() {
		snap, ok := loadSnapshot()
		if !ok {
			snap = Snapshot{}
		}
		snap.Active = env.Items
		if err := saveSnapshot(snap); err != nil {
			slog.Warn("httpapi: active snapshot save failed", "err", err)
		}
	}()
	return a.toDomain(env.Items, ""), nil
}

// Get returns the in-progress session for the given (user, project) pair.
// userID is ignored — the server scopes the response to the bearer token.
func (a *ActiveSessions) Get(_, projectID string) (domain.ActiveSession, error) {
	all, err := a.ListByUser("")
	if err != nil {
		return domain.ActiveSession{}, err
	}
	for _, act := range all {
		if act.ProjectID == projectID {
			return act, nil
		}
	}
	return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
}

// Upsert is unsupported — use WorktimeMachine.Start/Stop instead.
func (a *ActiveSessions) Upsert(_ domain.ActiveSession) error {
	return errors.New("httpapi: ActiveSessions sind read-only — WorktimeMachine benutzen")
}

// Delete is unsupported — use WorktimeMachine.Stop instead.
func (a *ActiveSessions) Delete(_, _ string) error {
	return errors.New("httpapi: ActiveSessions sind read-only — WorktimeMachine benutzen")
}

// — helpers -------------------------------------------------------------------

func (a *ActiveSessions) toDomain(dtos []activeDTO, userID string) []domain.ActiveSession {
	out := make([]domain.ActiveSession, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, activeFromDTO(d, userID))
	}
	return out
}
