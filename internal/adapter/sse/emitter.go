package sse

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Emitter publishes ev over the bus and, for loggable mutations, persists an
// activity entry then publishes EventActivityLogged for live refresh.
type Emitter struct {
	bus   *Bus
	store ports.ActivityStore
	ids   ports.IDGen
	clock ports.Clock
}

func NewEmitter(bus *Bus, store ports.ActivityStore, ids ports.IDGen, clock ports.Clock) *Emitter {
	return &Emitter{bus: bus, store: store, ids: ids, clock: clock}
}

func (e *Emitter) Emit(ctx context.Context, ev domain.Event) {
	e.bus.Publish(ev)
	entry, ok := activityFor(ctx, ev)
	if !ok {
		return
	}
	entry.ID = e.ids.NewID()
	entry.At = e.clock.Now()
	if err := e.store.Append(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "activity: append failed", "kind", entry.Kind, "err", err)
		return
	}
	e.bus.Publish(domain.Event{Type: domain.EventActivityLogged, UserID: ev.UserID})
}
