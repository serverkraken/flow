package sse_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
)

func TestBusCarriesNodeMoved(t *testing.T) {
	t.Parallel()
	b := sse.NewBus()
	ch, cancel := b.Subscribe("u1")
	defer cancel()

	b.Publish(domain.Event{Type: domain.EventNodeMoved, UserID: "u1", Data: map[string]any{"id": "n1", "parentId": "p1"}})
	select {
	case ev := <-ch:
		if ev.Type != domain.EventNodeMoved || ev.Data["id"] != "n1" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}
}
