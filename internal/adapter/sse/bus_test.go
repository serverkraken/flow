package sse_test

import (
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
)

func TestBusDeliversToSubscriberOfSameUser(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("user-1")
	defer cancel()

	b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "user-1"})

	select {
	case ev := <-ch:
		if ev.Type != domain.EventSessionStarted {
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
	b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "user-2"})
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
	b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "u"})
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestBusCapsPerUserEvictingOldest(t *testing.T) {
	b := sse.NewBus()
	b.SetMaxPerUser(3)
	var chans []<-chan domain.Event
	var cancels []func()
	for i := 0; i < 3; i++ {
		ch, cancel := b.Subscribe("u")
		chans = append(chans, ch)
		cancels = append(cancels, cancel)
	}
	// 4th subscription is over the cap → must evict the oldest (chans[0]).
	ch4, cancel4 := b.Subscribe("u")
	defer cancel4()
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	// oldest channel is closed by eviction.
	select {
	case _, ok := <-chans[0]:
		if ok {
			t.Fatal("oldest subscriber channel should be closed after eviction")
		}
	case <-time.After(time.Second):
		t.Fatal("oldest channel not closed — no eviction happened")
	}
	// the newest still receives events.
	b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "u"})
	select {
	case ev := <-ch4:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("wrong event %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("newest subscriber should still receive events")
	}
	// a still-live middle subscriber (chans[1]) was NOT evicted.
	select {
	case ev := <-chans[1]:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("wrong event %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("second subscriber should still be live")
	}
}

func TestBusCapIsolatesUsers(t *testing.T) {
	b := sse.NewBus()
	b.SetMaxPerUser(2)
	chA, cancelA := b.Subscribe("a")
	defer cancelA()
	// Fill user b to its cap and beyond; eviction must not touch user a.
	for i := 0; i < 3; i++ {
		_, _ = b.Subscribe("b")
	}
	b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "a"})
	select {
	case ev := <-chA:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("wrong event %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("user a must be unaffected by user b eviction")
	}
}

func TestBusConcurrentPublishSubscribeCancel(t *testing.T) {
	b := sse.NewBus()
	var wg sync.WaitGroup
	// publishers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				b.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: "u"})
			}
		}()
	}
	// subscribers that subscribe, drain a little, then cancel
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ch, cancel := b.Subscribe("u")
				select {
				case <-ch:
				default:
				}
				cancel()
				cancel() // double-cancel must be safe
			}
		}()
	}
	wg.Wait()
}
