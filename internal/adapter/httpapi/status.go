package httpapi

import (
	"sync"
	"time"
)

type ConnState int

const (
	StateUnknown ConnState = iota
	StateOnline
	StateOffline
	StateLoggedOut
	StateNotConfigured
	StateOutdated // Client < min_client_version
)

type StatusSnapshot struct {
	State         ConnState
	Host          string    // Server-Host für die Statuszeile
	LastFetched   time.Time // jüngster erfolgreicher Read (für "Stand 14:32")
	ServerVersion string
}

// Status ist von allen Resource-Adaptern geteilt; UI liest Snapshot(),
// Änderungen wecken den Changed-Kanal (coalesced, cap 1).
type Status struct {
	mu      sync.Mutex
	snap    StatusSnapshot
	changed chan struct{}
}

func newStatus() *Status { return &Status{changed: make(chan struct{}, 1)} }

func (s *Status) Snapshot() StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

func (s *Status) Changed() <-chan struct{} { return s.changed }

func (s *Status) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Status) set(mut func(*StatusSnapshot)) {
	s.mu.Lock()
	before := s.snap
	mut(&s.snap)
	after := s.snap
	s.mu.Unlock()
	if before != after {
		s.notify()
	}
}

func (s *Status) setOnline(host string) {
	s.set(func(sn *StatusSnapshot) {
		if sn.State != StateOutdated { // Outdated bleibt kleben bis Neustart
			sn.State = StateOnline
		}
		sn.Host = host
		sn.LastFetched = time.Now()
	})
}
func (s *Status) setOffline()   { s.set(func(sn *StatusSnapshot) { sn.State = StateOffline }) }
func (s *Status) setLoggedOut() { s.set(func(sn *StatusSnapshot) { sn.State = StateLoggedOut }) }

// StatusOf exponiert den Tracker für main.go/TUI.
func (c *Client) StatusOf() *Status { return c.status }
