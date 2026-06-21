// Package sse provides an in-process EventBus and SSE writing helpers.
package sse

import (
	"sync"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// defaultMaxPerUser bounds concurrent SSE subscriptions per user. It caps the
// blast radius of a misbehaving or malicious authenticated client that opens
// many /api/v1/events streams (each costs a goroutine + buffered channel),
// while staying generous enough for legitimate multi-tab / multi-device use.
const defaultMaxPerUser = 32

type subscriber struct {
	userID string
	seq    uint64 // monotonic insertion order, used to evict the oldest
	ch     chan domain.Event
	once   sync.Once
}

// close shuts the subscriber's channel exactly once. Both cancel() and
// eviction route through here, so a racing double-close cannot panic.
func (s *subscriber) close() { s.once.Do(func() { close(s.ch) }) }

var _ ports.EventBus = (*Bus)(nil)

// Bus is a thread-safe, in-process pub/sub that routes domain.Event by UserID.
// It implements ports.EventBus.
type Bus struct {
	mu         sync.Mutex
	subs       map[*subscriber]struct{}
	seq        uint64
	maxPerUser int
}

// NewBus returns an initialized *Bus ready for use.
func NewBus() *Bus {
	return &Bus{subs: map[*subscriber]struct{}{}, maxPerUser: defaultMaxPerUser}
}

// SetMaxPerUser overrides the per-user subscription cap. A value <= 0 is
// ignored. Intended for configuration and tests.
func (b *Bus) SetMaxPerUser(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.maxPerUser = n
	b.mu.Unlock()
}

// Subscribe registers a listener for userID and returns a receive-only channel
// plus a cancel func. Calling cancel unsubscribes and closes the channel;
// subsequent calls to cancel are no-ops.
//
// To bound resource use, at most maxPerUser subscriptions are kept alive per
// user: registering one over the cap evicts that user's oldest subscription
// (its channel is closed, so its SSE handler returns).
func (b *Bus) Subscribe(userID string) (<-chan domain.Event, func()) {
	b.mu.Lock()
	b.seq++
	s := &subscriber{userID: userID, seq: b.seq, ch: make(chan domain.Event, 16)}
	b.subs[s] = struct{}{}
	for evicted := b.evictOldestIfOverCapLocked(userID); evicted != nil; evicted = b.evictOldestIfOverCapLocked(userID) {
		evicted.close()
	}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, s)
			b.mu.Unlock()
			s.close()
		})
	}
	return s.ch, cancel
}

// evictOldestIfOverCapLocked removes and returns the oldest subscriber for
// userID when that user is over the cap, else nil. Caller holds b.mu; the
// returned subscriber must be closed by the caller (outside or after the loop)
// — closing is cheap and lock-free, so we close under the lock for simplicity.
func (b *Bus) evictOldestIfOverCapLocked(userID string) *subscriber {
	var count int
	var oldest *subscriber
	for s := range b.subs {
		if s.userID != userID {
			continue
		}
		count++
		if oldest == nil || s.seq < oldest.seq {
			oldest = s
		}
	}
	if count <= b.maxPerUser || oldest == nil {
		return nil
	}
	delete(b.subs, oldest)
	return oldest
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
