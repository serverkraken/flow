package sse

// broadcaster_test.go — covers the per-user fan-out invariants:
//
//   - Subscribe + Publish → receive
//   - Two subscribers same user → both receive
//   - Cross-user isolation: A's subscriber doesn't see B's event
//   - Slow subscriber: Publish doesn't block, drops on full buffer
//   - Cancel: subsequent publishes don't deliver
//   - PublishAll: every user's subscribers receive

import (
	"sync"
	"testing"
	"time"
)

// recvWithin returns the next event on ch or fails the test after timeout.
// Centralises the "did this fire?" assertion so test bodies stay focused
// on the invariant they exercise.
func recvWithin(t *testing.T, ch <-chan Event, d time.Duration) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed before event arrived")
		}
		return ev
	case <-time.After(d):
		t.Fatalf("timed out waiting for event after %s", d)
		return Event{}
	}
}

// mustNotRecv asserts no event arrives on ch within d. Used to verify
// cross-user isolation and post-cancel silence.
func mustNotRecv(t *testing.T, ch <-chan Event, d time.Duration) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			// Channel closed — that's fine for the "no delivery" assertion.
			return
		}
		t.Fatalf("unexpected event delivered: %+v", ev)
	case <-time.After(d):
		// expected — no event
	}
}

func TestBroadcaster_Subscribe_Publish_Receive(t *testing.T) {
	t.Parallel()
	b := New()
	ch, cancel := b.Subscribe("user-a")
	t.Cleanup(cancel)

	b.Publish("user-a", Event{Type: "session.started", Data: map[string]string{"id": "s1"}})

	got := recvWithin(t, ch, 200*time.Millisecond)
	if got.Type != "session.started" {
		t.Errorf("event type: got %q, want %q", got.Type, "session.started")
	}
}

func TestBroadcaster_TwoSubscribersSameUser_BothReceive(t *testing.T) {
	t.Parallel()
	b := New()
	ch1, cancel1 := b.Subscribe("user-a")
	t.Cleanup(cancel1)
	ch2, cancel2 := b.Subscribe("user-a")
	t.Cleanup(cancel2)

	b.Publish("user-a", Event{Type: "project.created"})

	ev1 := recvWithin(t, ch1, 200*time.Millisecond)
	ev2 := recvWithin(t, ch2, 200*time.Millisecond)
	if ev1.Type != "project.created" || ev2.Type != "project.created" {
		t.Errorf("both subscribers must receive: got %q / %q", ev1.Type, ev2.Type)
	}
}

func TestBroadcaster_CrossUser_Isolation(t *testing.T) {
	t.Parallel()
	b := New()
	chA, cancelA := b.Subscribe("user-a")
	t.Cleanup(cancelA)
	chB, cancelB := b.Subscribe("user-b")
	t.Cleanup(cancelB)

	b.Publish("user-a", Event{Type: "session.updated"})

	// A must receive.
	gotA := recvWithin(t, chA, 200*time.Millisecond)
	if gotA.Type != "session.updated" {
		t.Errorf("user A event type: got %q", gotA.Type)
	}
	// B MUST NOT.
	mustNotRecv(t, chB, 100*time.Millisecond)
}

func TestBroadcaster_SlowSubscriber_DropsOnFullBuffer(t *testing.T) {
	t.Parallel()
	b := New()
	_, cancel := b.Subscribe("slow")
	t.Cleanup(cancel)

	// Fill the buffer + overflow. With subscriberBufferSize = 16 we publish
	// 32 events; even if some squeeze through during scheduling, the
	// dropped ones must NOT block — the whole loop must complete in well
	// under the timeout below.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 32; i++ {
			b.Publish("slow", Event{Type: "session.updated", Data: i})
		}
		close(done)
	}()
	select {
	case <-done:
		// expected — Publish never blocked despite the consumer never draining
	case <-time.After(1 * time.Second):
		t.Fatal("Publish blocked on slow subscriber — drop policy broken")
	}
}

func TestBroadcaster_Cancel_StopsDelivery(t *testing.T) {
	t.Parallel()
	b := New()
	ch, cancel := b.Subscribe("user-a")

	// Receive one event to confirm subscription works.
	b.Publish("user-a", Event{Type: "tick"})
	recvWithin(t, ch, 200*time.Millisecond)

	// Cancel, then publish again — the new event must NOT arrive on ch
	// (the channel is closed after cancel; the recv loop in the SSE
	// handler relies on ctx.Done so the closed channel is fine).
	cancel()
	b.Publish("user-a", Event{Type: "session.updated"})

	// Drain any already-closed-channel signal. A closed channel yields
	// zero value + ok=false; mustNotRecv treats that as "no delivery".
	mustNotRecv(t, ch, 100*time.Millisecond)
}

func TestBroadcaster_PublishAll_ReachesAllUsers(t *testing.T) {
	t.Parallel()
	b := New()
	chA, cancelA := b.Subscribe("user-a")
	t.Cleanup(cancelA)
	chB, cancelB := b.Subscribe("user-b")
	t.Cleanup(cancelB)
	chC, cancelC := b.Subscribe("user-c")
	t.Cleanup(cancelC)

	b.PublishAll(Event{Type: "tick", Data: int64(42)})

	for _, ch := range []<-chan Event{chA, chB, chC} {
		got := recvWithin(t, ch, 200*time.Millisecond)
		if got.Type != "tick" {
			t.Errorf("PublishAll: got %q, want tick", got.Type)
		}
	}
}

// TestBroadcaster_PublishEmptyUser_NoOp ensures defensive handling of an
// empty userID — should silently no-op rather than spam any "" subscriber
// the caller might have accidentally subscribed.
func TestBroadcaster_PublishEmptyUser_NoOp(t *testing.T) {
	t.Parallel()
	b := New()
	ch, cancel := b.Subscribe("user-a")
	t.Cleanup(cancel)
	b.Publish("", Event{Type: "ghost"})
	mustNotRecv(t, ch, 50*time.Millisecond)
}

// TestBroadcaster_ConcurrentSubscribeCancel ensures the map operations
// don't race. -race must catch any drift in the lock scope here.
func TestBroadcaster_ConcurrentSubscribeCancel(t *testing.T) {
	t.Parallel()
	b := New()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, cancel := b.Subscribe("u")
			b.Publish("u", Event{Type: "x"})
			// Drain at most one event so the buffer drains before cancel.
			select {
			case <-ch:
			case <-time.After(50 * time.Millisecond):
			}
			cancel()
		}()
	}
	wg.Wait()
}
