package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Machine implements ports.WorktimeMachine via the server bearer API.
// Start/Stop/Pause/Resume/CorrectStart are synchronous POST calls that
// mutate the server-side statemachine; after success the Active + Sessions
// caches are invalidated so the next read reflects the new state.
type Machine struct {
	c        *Client
	active   *ActiveSessions // invalidated after writes
	sessions *Sessions       // invalidated after Stop
}

// NewMachine constructs a Machine that shares caches with the given adapters.
func NewMachine(c *Client, active *ActiveSessions, sessions *Sessions) *Machine {
	return &Machine{c: c, active: active, sessions: sessions}
}

var _ ports.WorktimeMachine = (*Machine)(nil)

// Start posts to /worktime/active/start and returns the new active session.
func (m *Machine) Start(projectID, tag, note string) (domain.ActiveSession, error) {
	body := struct {
		ProjectID string `json:"project_id"`
		Tag       string `json:"tag,omitempty"`
		Note      string `json:"note,omitempty"`
	}{ProjectID: projectID, Tag: tag, Note: note}
	var dto activeDTO
	err := m.c.doJSON(context.Background(), http.MethodPost,
		"/api/v1/worktime/active/start", body, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusConflict {
			return domain.ActiveSession{}, ports.ErrActiveSessionConflict
		}
		return domain.ActiveSession{}, err
	}
	m.active.cache.invalidate()
	return activeFromDTO(dto, ""), nil
}

// Stop posts to /worktime/active/stop and returns the finished session.
func (m *Machine) Stop(projectID string) (domain.Session, error) {
	body := struct {
		ProjectID string `json:"project_id"`
	}{ProjectID: projectID}
	var dto sessionDTO
	err := m.c.doJSON(context.Background(), http.MethodPost,
		"/api/v1/worktime/active/stop", body, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return domain.Session{}, ports.ErrActiveSessionNotFound
		}
		return domain.Session{}, err
	}
	m.active.cache.invalidate()
	m.sessions.cache.invalidate()
	s, err := sessionFromDTO(dto, "")
	return s, err
}

// Pause posts to /worktime/active/pause.
func (m *Machine) Pause(projectID string) (domain.ActiveSession, error) {
	return m.pauseResume("/api/v1/worktime/active/pause", projectID)
}

// Resume posts to /worktime/active/resume.
func (m *Machine) Resume(projectID string) (domain.ActiveSession, error) {
	return m.pauseResume("/api/v1/worktime/active/resume", projectID)
}

func (m *Machine) pauseResume(path, projectID string) (domain.ActiveSession, error) {
	body := struct {
		ProjectID string `json:"project_id"`
	}{ProjectID: projectID}
	var dto activeDTO
	err := m.c.doJSON(context.Background(), http.MethodPost, path, body, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
		}
		return domain.ActiveSession{}, err
	}
	m.active.cache.invalidate()
	return activeFromDTO(dto, ""), nil
}

// CorrectStart posts to /worktime/active/correct.
func (m *Machine) CorrectStart(projectID string, startedAt time.Time) (domain.ActiveSession, error) {
	body := struct {
		ProjectID string    `json:"project_id"`
		StartedAt time.Time `json:"started_at"`
	}{ProjectID: projectID, StartedAt: startedAt}
	var dto activeDTO
	err := m.c.doJSON(context.Background(), http.MethodPost,
		"/api/v1/worktime/active/correct", body, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
		}
		return domain.ActiveSession{}, err
	}
	m.active.cache.invalidate()
	return activeFromDTO(dto, ""), nil
}
