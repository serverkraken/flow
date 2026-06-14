// Package testutil provides in-memory fakes for the ports, for use in tests.
package testutil

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type FakeClock struct{ T time.Time }

func (c FakeClock) Now() time.Time { return c.T }

type FakeIDGen struct {
	mu sync.Mutex
	n  int
}

func (g *FakeIDGen) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "id-" + itoa(g.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type FakeUserStore struct {
	mu    sync.Mutex
	bySub map[string]domain.User
}

func NewFakeUserStore() *FakeUserStore { return &FakeUserStore{bySub: map[string]domain.User{}} }

func (s *FakeUserStore) UpsertBySub(_ context.Context, u domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySub[u.OIDCSub] = u
	return u, nil
}

func (s *FakeUserStore) GetBySub(_ context.Context, sub string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.bySub[sub]
	if !ok {
		return domain.User{}, ports.ErrUserNotFound
	}
	return u, nil
}

type FakeVerifier struct {
	ID  ports.Identity
	Err error
}

func (v FakeVerifier) Verify(context.Context, string) (ports.Identity, error) {
	return v.ID, v.Err
}

// FakeProjectStore is an in-memory ports.ProjectStore.
type FakeProjectStore struct {
	mu sync.Mutex
	m  map[string]domain.Project // keyed by id
}

func NewFakeProjectStore() *FakeProjectStore { return &FakeProjectStore{m: map[string]domain.Project{}} }

func (s *FakeProjectStore) Create(_ context.Context, p domain.Project) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p.ID] = p
	return p, nil
}

func (s *FakeProjectStore) List(_ context.Context, ownerID string) ([]domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Project
	for _, p := range s.m {
		if p.OwnerID == ownerID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *FakeProjectStore) Get(_ context.Context, ownerID, id string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, nil
}

// FakeSessionStore is an in-memory ports.SessionStore enforcing one running
// session per owner, like the Postgres partial index.
type FakeSessionStore struct {
	mu sync.Mutex
	m  map[string]domain.WorkSession // keyed by id
}

func NewFakeSessionStore() *FakeSessionStore {
	return &FakeSessionStore{m: map[string]domain.WorkSession{}}
}

func (s *FakeSessionStore) Create(_ context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ws.Stop == nil {
		for _, e := range s.m {
			if e.OwnerID == ws.OwnerID && e.Stop == nil {
				return domain.WorkSession{}, errors.New("fake: running session exists")
			}
		}
	}
	s.m[ws.ID] = ws
	return ws, nil
}

func (s *FakeSessionStore) Running(_ context.Context, ownerID string) (domain.WorkSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.m {
		if e.OwnerID == ownerID && e.Stop == nil {
			return e, true, nil
		}
	}
	return domain.WorkSession{}, false, nil
}

func (s *FakeSessionStore) Stop(_ context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID || e.Stop != nil {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.Stop = &stop
	e.ProjectID = projectID
	s.m[id] = e
	return e, nil
}

func (s *FakeSessionStore) List(_ context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID && !e.Start.Before(since) {
			out = append(out, e)
		}
	}
	return out, nil
}
