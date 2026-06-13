package sse_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
)

func TestBusDeliversToSubscriberOfSameUser(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("user-1")
	defer cancel()

	b.Publish(domain.Event{Type: domain.EventPing, UserID: "user-1"})

	select {
	case ev := <-ch:
		if ev.Type != domain.EventPing {
			t.Fatalf("wrong event: %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusIsolatesUsers(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("user-1")
	defer cancel()
	b.Publish(domain.Event{Type: domain.EventPing, UserID: "user-2"})
	select {
	case <-ch:
		t.Fatal("user-1 must not receive user-2 events")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCancelUnsubscribes(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("u")
	cancel()
	b.Publish(domain.Event{Type: domain.EventPing, UserID: "u"})
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
}
