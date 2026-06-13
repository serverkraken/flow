package httpapi

// Sessions implements ports.SessionStore against the bearer API.
//
// Server scopes all reads/writes to the authenticated user — the userID
// parameter on each method is accepted for interface compliance but ignored.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Sessions implements ports.SessionStore via the server bearer API.
type Sessions struct {
	c        *Client
	cache    resourceCache[[]sessionDTO]
	versions map[string]int64 // id → last-seen version, used for If-Match
	mu       sync.Mutex       // guards versions map
}

// NewSessions constructs a Sessions adapter backed by c.
func NewSessions(c *Client) *Sessions {
	s := &Sessions{c: c, versions: make(map[string]int64)}
	if snap, ok := loadSnapshot(); ok {
		s.cache.put(snap.Sessions)
		for _, d := range snap.Sessions {
			s.versions[d.ID] = d.Version
		}
	}
	return s
}

var _ ports.SessionStore = (*Sessions)(nil)

// Invalidate forces a cache miss on the next read. Called by the SSE events
// client in main.go's invalidate dispatch when a "worktime" changed event
// arrives from the server.
func (s *Sessions) Invalidate() { s.cache.invalidate() }

// Load returns all sessions for the authenticated user.
// userID is ignored — the server scopes the response to the bearer token.
func (s *Sessions) Load(_ string) ([]domain.Session, error) {
	if cached, ok := s.cache.get(); ok {
		return s.toDomain(cached, ""), nil
	}
	var env itemsEnvelope[sessionDTO]
	err := s.c.doJSON(context.Background(), http.MethodGet,
		"/api/v1/worktime/sessions?from=2000-01-01&to=2100-12-31",
		nil, -1, &env)
	if err != nil {
		if fb, ok := s.cache.fallback(); ok {
			return s.toDomain(fb, ""), nil
		}
		return nil, err
	}
	s.cache.put(env.Items)
	s.updateVersions(env.Items)
	go func() {
		snap, ok := loadSnapshot()
		if !ok {
			snap = Snapshot{}
		}
		snap.Sessions = env.Items
		if err := saveSnapshot(snap); err != nil {
			slog.Warn("httpapi: sessions snapshot save failed", "err", err)
		}
	}()
	return s.toDomain(env.Items, ""), nil
}

// LoadFiltered returns sessions for which keep returns true.
// userID is ignored — the server scopes the response to the bearer token.
func (s *Sessions) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := s.Load("")
	if err != nil {
		return nil, err
	}
	out := make([]domain.Session, 0, len(all))
	for _, sess := range all {
		if keep(sess) {
			out = append(out, sess)
		}
	}
	return out, nil
}

// Upsert inserts or updates a single session.
// userID is ignored — the server scopes writes to the bearer token.
func (s *Sessions) Upsert(sess domain.Session) error {
	body := sessionWriteDTO{
		ProjectID: sess.ProjectID,
		StartedAt: sess.Start,
		StoppedAt: sess.Stop,
		Tag:       sess.Tag,
		Note:      sess.Note,
	}
	s.mu.Lock()
	v, exists := s.versions[sess.ID]
	s.mu.Unlock()

	var out sessionDTO
	var err error
	if exists && v > 0 {
		// Update existing session with If-Match
		err = s.c.doJSON(context.Background(), http.MethodPut,
			fmt.Sprintf("/api/v1/worktime/sessions/%s", sess.ID),
			body, v, &out)
	} else {
		// Create new session — server assigns ID
		err = s.c.doJSON(context.Background(), http.MethodPost,
			"/api/v1/worktime/sessions",
			body, -1, &out)
	}
	if err != nil {
		switch statusCode(err) {
		case http.StatusPreconditionFailed:
			return ports.ErrSessionVersionConflict
		case http.StatusNotFound:
			return ports.ErrSessionNotFound
		}
		return err
	}
	s.mu.Lock()
	s.versions[out.ID] = out.Version
	s.mu.Unlock()
	s.cache.invalidate()
	return nil
}

// UpsertBatch submits a bulk upsert to the server.
// userID is ignored — the server scopes writes to the bearer token.
func (s *Sessions) UpsertBatch(sessions []domain.Session) error {
	type bulkBody struct {
		Sessions []sessionWriteDTO `json:"sessions"`
	}
	dtos := make([]sessionWriteDTO, 0, len(sessions))
	for _, sess := range sessions {
		dtos = append(dtos, sessionWriteDTO{
			ID:        sess.ID,
			ProjectID: sess.ProjectID,
			StartedAt: sess.Start,
			StoppedAt: sess.Stop,
			Tag:       sess.Tag,
			Note:      sess.Note,
		})
	}
	err := s.c.doJSON(context.Background(), http.MethodPost,
		"/api/v1/worktime/sessions:bulk",
		bulkBody{Sessions: dtos}, -1, nil)
	if err != nil {
		return err
	}
	s.cache.invalidate()
	return nil
}

// Delete removes a session by ID.
// userID is ignored — the server scopes writes to the bearer token.
func (s *Sessions) Delete(_, id string) error {
	s.mu.Lock()
	v, ok := s.versions[id]
	s.mu.Unlock()
	if !ok {
		// Populate versions map first
		if _, err := s.Load(""); err != nil {
			return err
		}
		s.mu.Lock()
		v, ok = s.versions[id]
		s.mu.Unlock()
		if !ok {
			return ports.ErrSessionNotFound
		}
	}
	err := s.c.doJSON(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/worktime/sessions/%s", id),
		nil, v, nil)
	if err != nil {
		switch statusCode(err) {
		case http.StatusNotFound:
			return ports.ErrSessionNotFound
		case http.StatusPreconditionFailed:
			return ports.ErrSessionVersionConflict
		}
		return err
	}
	s.mu.Lock()
	delete(s.versions, id)
	s.mu.Unlock()
	s.cache.invalidate()
	return nil
}

// — helpers -------------------------------------------------------------------

func (s *Sessions) updateVersions(dtos []sessionDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range dtos {
		s.versions[d.ID] = d.Version
	}
}

func (s *Sessions) toDomain(dtos []sessionDTO, userID string) []domain.Session {
	out := make([]domain.Session, 0, len(dtos))
	for _, d := range dtos {
		sess, err := sessionFromDTO(d, userID)
		if err != nil {
			slog.Warn("httpapi: skipping malformed session", "id", d.ID, "err", err)
			continue
		}
		out = append(out, sess)
	}
	return out
}
