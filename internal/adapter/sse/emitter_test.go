package sse_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// fakeStore records Append calls for assertion.
type fakeStore struct {
	entries []domain.ActivityEntry
}

func (f *fakeStore) Append(_ context.Context, e domain.ActivityEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeStore) ListPage(_ context.Context, _ string, _ []string, _ *string, _, _ int) ([]domain.ActivityEntry, int, error) {
	return nil, 0, nil
}

func (f *fakeStore) DistinctActors(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

var _ ports.ActivityStore = (*fakeStore)(nil)

// fakeClock returns a fixed instant.
type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

// fakeIDs returns a fixed id.
type fakeIDs struct{ id string }

func (f fakeIDs) NewID() string { return f.id }

// drain collects up to n events from ch with a short timeout, returning as many
// as arrived before the timeout.
func drain(ch <-chan domain.Event, n int) []domain.Event {
	out := make([]domain.Event, 0, n)
	deadline := time.After(200 * time.Millisecond)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestEmitter_loggable(t *testing.T) {
	bus := sse.NewBus()
	store := &fakeStore{}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	emitter := sse.NewEmitter(bus, store, fakeIDs{"id-1"}, fakeClock{now})

	const userID = "user-1"
	ch, cancel := bus.Subscribe(userID)
	defer cancel()

	ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Agent, Ref: "claude-code"})
	emitter.Emit(ctx, domain.Event{
		Type:   domain.EventDocumentUpdated,
		UserID: userID,
		Data:   map[string]any{"id": "d1", "title": "Foo"},
	})

	evts := drain(ch, 2)
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(evts), evts)
	}
	if evts[0].Type != domain.EventDocumentUpdated {
		t.Errorf("first event: want %q, got %q", domain.EventDocumentUpdated, evts[0].Type)
	}
	if evts[1].Type != domain.EventActivityLogged {
		t.Errorf("second event: want %q, got %q", domain.EventActivityLogged, evts[1].Type)
	}

	if len(store.entries) != 1 {
		t.Fatalf("expected 1 store entry, got %d", len(store.entries))
	}
	e := store.entries[0]
	if e.ActorRef != "claude-code" {
		t.Errorf("ActorRef: want %q, got %q", "claude-code", e.ActorRef)
	}
	if e.Label == nil || *e.Label != "Foo" {
		t.Errorf("Label: want \"Foo\", got %v", e.Label)
	}
	if e.ID != "id-1" {
		t.Errorf("ID: want %q, got %q", "id-1", e.ID)
	}
	if !e.At.Equal(now) {
		t.Errorf("At: want %v, got %v", now, e.At)
	}
}

func TestEmitter_settingsChanged_notLogged(t *testing.T) {
	bus := sse.NewBus()
	store := &fakeStore{}
	emitter := sse.NewEmitter(bus, store, fakeIDs{"x"}, fakeClock{time.Now()})

	const userID = "user-2"
	ch, cancel := bus.Subscribe(userID)
	defer cancel()

	ctx := context.Background()
	emitter.Emit(ctx, domain.Event{Type: domain.EventSettingsChanged, UserID: userID})

	evts := drain(ch, 2)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event (settings.changed only), got %d: %v", len(evts), evts)
	}
	if evts[0].Type != domain.EventSettingsChanged {
		t.Errorf("event type: want %q, got %q", domain.EventSettingsChanged, evts[0].Type)
	}
	if len(store.entries) != 0 {
		t.Errorf("store must be empty for settings.changed, got %d entries", len(store.entries))
	}
}
