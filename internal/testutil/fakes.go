// Package testutil provides in-memory fakes for the ports, for use in tests.
package testutil

import (
	"context"
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
