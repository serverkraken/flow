package worktime

// White-box test for tickInterval: pins the Cluster-B review fix that
// scheduleTick consults the injected Clock port instead of wall-clock
// time.Now(). Wall-clock-driven branch selection makes the fast/slow
// transition undeterministic under a fake clock in tests.

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

type tickFakeClock struct{ t time.Time }

func (c *tickFakeClock) Now() time.Time { return c.t }

func TestTickInterval_FastWithinFirstMinute(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	clock := &tickFakeClock{t: start.Add(30 * time.Second)}
	m := New(theme.Default, Deps{Clock: clock})
	h := m.subs[tabHeute].(heute)
	h.day = domain.Day{Active: &start}
	m.subs[tabHeute] = h
	if got := m.tickInterval(); got != tickFast {
		t.Errorf("within first minute: got %v, want tickFast (%v)", got, tickFast)
	}
}

func TestTickInterval_SlowPastFirstMinute(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	clock := &tickFakeClock{t: start.Add(90 * time.Second)}
	m := New(theme.Default, Deps{Clock: clock})
	h := m.subs[tabHeute].(heute)
	h.day = domain.Day{Active: &start}
	m.subs[tabHeute] = h
	if got := m.tickInterval(); got != tickSlow {
		t.Errorf("past first minute: got %v, want tickSlow (%v)", got, tickSlow)
	}
}

func TestTickInterval_SlowWhenNoActiveSession(t *testing.T) {
	t.Parallel()
	clock := &tickFakeClock{t: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)}
	m := New(theme.Default, Deps{Clock: clock})
	if got := m.tickInterval(); got != tickSlow {
		t.Errorf("no active session: got %v, want tickSlow (%v)", got, tickSlow)
	}
}
