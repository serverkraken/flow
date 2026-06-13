// Package sse provides an in-process EventBus and SSE writing helpers.
package sse

import (
	"sync"

	"github.com/serverkraken/flow/internal/domain"
)

type subscriber struct {
	userID string
	ch     chan domain.Event
}

// Bus is a thread-safe, in-process pub/sub that routes domain.Event by UserID.
// It implements ports.EventBus.
type Bus struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

// NewBus returns an initialized *Bus ready for use.
func NewBus() *Bus { return &Bus{subs: map[*subscriber]struct{}{}} }

// Subscribe registers a listener for userID and returns a receive-only channel
// plus a cancel func. Calling cancel unsubscribes and closes the channel;
// subsequent calls to cancel are no-ops.
func (b *Bus) Subscribe(userID string) (<-chan domain.Event, func()) {
	s := &subscriber{userID: userID, ch: make(chan domain.Event, 16)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, s)
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Publish fans out ev to every subscriber whose userID matches ev.UserID.
// The send is non-blocking: a full subscriber buffer drops the event for that
// subscriber rather than stalling the publisher. This is intentional — the
// SSE client will full-refresh on reconnect.
func (b *Bus) Publish(ev domain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.userID != ev.UserID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}
