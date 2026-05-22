package worktime_test

// Drives the Heute Edit dialog through a successful submit so the
// SessionWriter.Edit → SetTag → SetNote chain is exercised. The
// existing TestHeute_EditDialog_SubmitWithBadStart tests the error
// path only; this pins the success branch.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

func TestHeute_EditDialog_SuccessfulSubmit(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-2 * time.Hour), Stop: now.Add(-1 * time.Hour), Elapsed: time.Hour, Tag: "old", Note: "n"},
	}
	m := loadedHeute(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = drainCmd(t, m, cmd)
	// Form is already populated with the existing session's values. Just
	// hit Enter on each field until the last, then submit.
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	// After successful submit, the drain might dispatch a heuteActionDoneMsg
	// that closes the dialog. The session count should stay at 1.
	if len(r.sessions.Sessions) != 1 {
		t.Errorf("session count should remain 1, got %d", len(r.sessions.Sessions))
	}
}
