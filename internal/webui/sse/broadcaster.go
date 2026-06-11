// Package sse provides a per-user event broadcaster for the WebUI's
// Server-Sent-Events stream. The shape is deliberately tiny:
//
//   - one Broadcaster instance, owned by main.go
//   - one Subscribe() per active SSE handler call → returns a receive-only
//     channel + an unsubscribe func
//   - Publish(userID, ev) fans out to every subscriber of that user
//   - PublishAll(ev) reaches every subscriber across every user (used by the
//     1Hz ticker)
//
// Slow subscribers are NEVER allowed to block publishers: every subscriber
// channel is small-buffered, and Publish uses a non-blocking select-default
// so a stuck consumer drops events instead of stalling the entire mutation
// path. The drop is logged at warn so an operator notices a wedged tab.
//
// Per-user fan-out (rather than global) means a session.updated event for
// user A never reaches user B's SSE stream — the dashboard partial swap
// would otherwise hit the WRONG user's row. Subscribe takes the userID
// from the authenticated context above this layer; cross-tenant leakage
// would require a bug in the SSE handler, not here.
package sse

import (
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// subscribersGauge tracks the number of live SSE subscribers across all
// users. Defined here (rather than in httpserver) so the gauge lives
// next to the only code that mutates it, avoiding a layering inversion
// where sse would have to import the httpserver adapter. The metric
// registers into prometheus's default registry on package init, so the
// /metrics handler (also default-registry) exposes it automatically.
// Prometheus collectors are package-global by design.
//
//nolint:gochecknoglobals
var subscribersGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "flow_sse_subscribers",
	Help: "Active SSE subscribers (Task 14 broadcaster).",
})

// subscriberBufferSize is the per-subscriber channel capacity. Small on
// purpose: SSE consumers should drain immediately. A backlog larger than
// this means the consumer is genuinely stuck (closed tab, frozen browser);
// dropping further events is preferable to blocking the publisher.
const subscriberBufferSize = 16

// Event is the wire shape for a single SSE message. Type maps to the SSE
// `event: <type>` line; Data is JSON-marshalled into the `data: <json>`
// line by the HTTP handler. Keep payloads small — the dashboard's
// "full-page refresh on session.* event" model means the data field is
// mostly metadata, not the rendered HTML.
type Event struct {
	Type string
	Data any
}

// Broadcaster fans events out to per-user subscriber channels. Zero value
// is NOT usable — call New().
type Broadcaster struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

// New constructs an empty broadcaster ready for Subscribe / Publish.
func New() *Broadcaster {
	return &Broadcaster{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a new subscriber for userID. Returns a receive-only
// channel and a cancel func that the caller MUST invoke (typically via
// `defer cancel()`) to free the subscription on disconnect.
//
// The returned channel has a small buffer so a publisher writing while the
// consumer is mid-flush doesn't drop the event. Once the buffer is full,
// Publish drops further events for THIS subscriber (other subscribers of
// the same user still receive).
func (b *Broadcaster) Subscribe(userID string) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBufferSize)
	b.mu.Lock()
	if b.subs[userID] == nil {
		b.subs[userID] = make(map[chan Event]struct{})
	}
	b.subs[userID][ch] = struct{}{}
	b.mu.Unlock()
	subscribersGauge.Inc()

	// cancel removes this subscriber from the registry. We intentionally
	// do NOT close(ch) here: Publish snapshots subscribers under the
	// lock and then sends OUTSIDE the lock, so a concurrent close
	// would race with that send (panic on closed channel). Subscribers
	// who care about shutdown use the request context's Done; the
	// channel itself just stops receiving events once unregistered and
	// gets garbage-collected with the closure.
	cancel := func() {
		// Decrement the gauge only when this channel was actually
		// still registered — guards against double-cancel (defer +
		// explicit) inflating the "subscribers leaving" rate.
		b.mu.Lock()
		removed := false
		if set, ok := b.subs[userID]; ok {
			if _, present := set[ch]; present {
				delete(set, ch)
				removed = true
			}
			if len(set) == 0 {
				delete(b.subs, userID)
			}
		}
		b.mu.Unlock()
		if removed {
			subscribersGauge.Dec()
		}
	}
	return ch, cancel
}

// Publish sends ev to every subscriber registered under userID. Returns
// immediately even if some subscribers are slow — drops the event for any
// channel whose buffer is full. Empty userID is a no-op (defensive — a
// missing user in the calling handler should have returned 401 long
// before reaching here).
func (b *Broadcaster) Publish(userID string, ev Event) {
	if userID == "" {
		return
	}
	// Copy subscribers under the lock, then send outside the lock so a
	// slow consumer's blocked send doesn't stall other subscribers (or
	// Subscribe/Cancel in flight). The non-blocking select-default below
	// makes "stall" theoretically impossible, but the copy keeps the
	// lock scope as small as possible.
	b.mu.Lock()
	set := b.subs[userID]
	subs := make([]chan Event, 0, len(set))
	for ch := range set {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// Slow subscriber — drop the event rather than blocking the
			// publisher. The browser will see a gap in the stream but
			// the mutation that triggered Publish already succeeded;
			// the next event (or a full reload) re-converges state.
			slog.Warn(
				"sse: dropping event for slow subscriber",
				slog.String("user_id", userID),
				slog.String("event_type", ev.Type),
			)
		}
	}
}

// PublishAll fans ev out to every subscriber across every user. Used by
// the 1Hz ticker so all open dashboards refresh their "läuft seit MM:SS"
// counter in lockstep. Like Publish, drops events for slow subscribers.
func (b *Broadcaster) PublishAll(ev Event) {
	b.mu.Lock()
	subs := make([]chan Event, 0)
	for _, set := range b.subs {
		for ch := range set {
			subs = append(subs, ch)
		}
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// Same drop-on-full policy as Publish. The ticker is
			// noisy by design (1/sec) — losing one tick is cheap; the
			// next one is ≤1 second away.
			slog.Warn(
				"sse: dropping broadcast event for slow subscriber",
				slog.String("event_type", ev.Type),
			)
		}
	}
}

// Changed publishes the generalized cross-client invalidation event
// (Spec §7 /events): every successful write fans out
// `changed {resource}` so TUI/MCP/Browser refetch the resource. The
// legacy fine-grained session.*/project.*/note.* events stay for the
// WebUI's own HTMX swaps — Changed is the cross-device contract.
func (b *Broadcaster) Changed(userID, resource string) {
	b.Publish(userID, Event{Type: "changed", Data: map[string]string{"resource": resource}})
}
