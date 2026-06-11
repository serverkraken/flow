// internal/webui/handlers/session_actions_pause_test.go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type fakePauseActive struct {
	active domain.ActiveSession
	paused bool
}

func (f *fakePauseActive) ListByUser(string) ([]domain.ActiveSession, error) {
	return []domain.ActiveSession{f.current()}, nil
}

func (f *fakePauseActive) Get(string, string) (domain.ActiveSession, error) { return f.current(), nil }

func (f *fakePauseActive) Start(string, string, time.Time, string, int64, string, string) (domain.ActiveSession, error) {
	return domain.ActiveSession{}, ports.ErrActiveSessionConflict
}

func (f *fakePauseActive) Stop(string, string, int64, string, string) (domain.Session, error) {
	return domain.Session{}, ports.ErrActiveSessionNotFound
}

func (f *fakePauseActive) Pause(string, string) (domain.ActiveSession, error) {
	f.paused = true
	return f.current(), nil
}

func (f *fakePauseActive) Resume(string, string) (domain.ActiveSession, error) {
	f.paused = false
	return f.current(), nil
}

func (f *fakePauseActive) current() domain.ActiveSession {
	a := f.active
	if f.paused {
		now := time.Now()
		a.PausedAt = &now
	}
	return a
}

type fakePauseProjects struct{}

func (fakePauseProjects) ListActive(string) ([]domain.Project, error) { return nil, nil }
func (fakePauseProjects) ListAll(string) ([]domain.Project, error)    { return nil, nil }
func (fakePauseProjects) GetBySlug(string, string) (domain.Project, error) {
	return domain.Project{}, ports.ErrProjectNotFound
}

func (fakePauseProjects) GetByID(string, string) (domain.Project, error) {
	return domain.Project{ID: "p1", Name: "Demo"}, nil
}

func (fakePauseProjects) EnsureBySlug(string, string, string) (domain.Project, error) {
	return domain.Project{}, nil
}

func (fakePauseProjects) Upsert(domain.Project, int64) (domain.Project, error) {
	return domain.Project{}, nil
}
func (fakePauseProjects) TouchLastUsed(string, string) error { return nil }
func (fakePauseProjects) Archive(string, string) error       { return nil }

type fakePauseClock struct{ t time.Time }

func (c fakePauseClock) Now() time.Time { return c.t }

func TestActivePauseResume_RendersBannerStates(t *testing.T) {
	t.Parallel()
	fake := &fakePauseActive{active: domain.ActiveSession{
		UserID: "u1", ProjectID: "p1", StartedAt: time.Now().Add(-30 * time.Minute), Version: 1,
	}}
	deps := SessionActionsDeps{
		Active:      fake,
		PauseResume: fake,
		Projects:    fakePauseProjects{},
		Clock:       fakePauseClock{t: time.Now()},
	}

	do := func(h http.Handler) string {
		req := httptest.NewRequest(http.MethodPost, "/worktime/active/pause", nil)
		req = req.WithContext(httpserver.WithUser(req.Context(), domain.User{ID: "u1"}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: %d", rec.Code)
		}
		return rec.Body.String()
	}

	body := do(NewActivePause(deps))
	if !strings.Contains(body, "data-paused") || !strings.Contains(body, "Weiter") {
		t.Errorf("paused banner: erwartet data-paused + Weiter-Button, got: %.300s", body)
	}

	body = do(NewActiveResume(deps))
	if strings.Contains(body, "data-paused") || !strings.Contains(body, "Pause") {
		t.Errorf("resumed banner: kein data-paused, Pause-Button erwartet, got: %.300s", body)
	}
}
