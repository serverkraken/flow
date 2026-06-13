package worktime_test

// TestHeute_SetTag_RoundTrip proves the tag dialog round-trips through
// SessionWriter.SetTag → Sessions.Upsert without panicking. This pins the
// fix that wired a server-backed SessionWriter in cmd/flow/main.go (A2).
// Prior to A2 the writer was nil in server mode, causing a nil-panic when
// the user opened the tag dialog and pressed Enter on a finished session.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

func TestHeute_SetTag_RoundTrip(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	// Seed one finished session for today.
	r.sessions.Sessions = []domain.Session{
		{
			Date:    now,
			Start:   now.Add(-2 * time.Hour),
			Stop:    now.Add(-1 * time.Hour),
			Elapsed: time.Hour,
			Tag:     "", // no tag yet
		},
	}
	m := loadedHeute(t, r)

	// Open the tag dialog with 't'.
	m, cmd := m.Update(tea.KeyPressMsg{Text: "t"})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("'t' should open the tag dialog (FilterActive=true)")
	}

	// Type a tag value.
	for _, ch := range "deep-work" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}

	// Submit with Enter — triggers submitDialog → sw.SetTag → Sessions.Upsert.
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)

	// The dialog should be closed after a successful submit.
	if m.(worktime.Model).FilterActive() {
		t.Error("dialog should be closed after successful tag submit")
	}

	// The backing store must now hold the updated tag.
	if len(r.sessions.Sessions) == 0 {
		t.Fatal("session store is empty — Upsert was not called")
	}
	got := r.sessions.Sessions[0].Tag
	if got != "deep-work" {
		t.Errorf("session Tag = %q, want %q", got, "deep-work")
	}
}
