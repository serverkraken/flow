package httpapi

import (
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// ConnState aliases ports.ConnState so httpapi-internal code and white-box
// tests keep using the short name without a ports import at every call site.
type ConnState = ports.ConnState

// Connection-state constants re-exported from ports so callers in this
// package (meta.go, status_test.go) compile without change.
const (
	StateUnknown       = ports.StateUnknown
	StateOnline        = ports.StateOnline
	StateOffline       = ports.StateOffline
	StateLoggedOut     = ports.StateLoggedOut
	StateNotConfigured = ports.StateNotConfigured
	StateOutdated      = ports.StateOutdated
)

// StatusSnapshot aliases ports.StatusSnapshot for httpapi-internal convenience.
type StatusSnapshot = ports.StatusSnapshot

// Status is shared across all resource adapters; the UI reads Snapshot(),
// state changes wake the Changed channel (coalesced, cap 1).
type Status struct {
	mu      sync.Mutex
	snap    StatusSnapshot
	changed chan struct{}
}

func newStatus() *Status { return &Status{changed: make(chan struct{}, 1)} }

// Snapshot returns a point-in-time copy of the current connection state.
func (s *Status) Snapshot() StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// Changed returns a channel that fires (coalesced) whenever the connection
// state changes. Suitable for wiring into a bubbletea listenForChanged Cmd.
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

// StatusOf exposes the tracker for main.go and the TUI.
func (c *Client) StatusOf() *Status { return c.status }
