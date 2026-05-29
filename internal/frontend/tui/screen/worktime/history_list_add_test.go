package worktime_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

func TestListAdd_OpensAndCancelsWithEsc(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "nachbuchen") {
		t.Errorf("list add dialog should render its title, got:\n%s", out)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if strings.Contains(strings.ToLower(m.View().Content), "nachbuchen") {
		t.Errorf("Esc should close the list add dialog")
	}
}

func TestListAdd_SeedsYesterdayDate(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	yesterday := r.clock.T.AddDate(0, 0, -1)
	if !strings.Contains(out, yesterday.Format("2006-01-02")) {
		t.Errorf("list add dialog should seed yesterday's date (%s), got:\n%s",
			yesterday.Format("2006-01-02"), out)
	}
}

func TestListAdd_SubmitCreatesSession(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	before := len(r.sessions.Sessions)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Tab past date (prefilled), land on start.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// Tab past start (prefilled 09:00), land on stop.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// Type a stop value.
	for _, ch := range "+2h" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab past tag and note.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// Enter on the last field → submit.
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	if len(r.sessions.Sessions) != before+1 {
		t.Errorf("after submit, session count: got %d want %d",
			len(r.sessions.Sessions), before+1)
	}
	if strings.Contains(strings.ToLower(m.View().Content), "nachbuchen") {
		t.Errorf("dialog should be closed after successful submit")
	}
}

func TestListAdd_BadDateKeepsDialog(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Clear the prefilled date.
	for i := 0; i < 12; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, ch := range "nope" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab through remaining fields, then Enter.
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	if !strings.Contains(strings.ToLower(m.View().Content), "nachbuchen") {
		t.Errorf("bad date should keep the dialog open, got:\n%s", m.View().Content)
	}
}

func TestListAdd_FutureDateRejected(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Clear the prefilled date and type a future date.
	for i := 0; i < 12; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	future := r.clock.T.AddDate(0, 0, 5).Format("2006-01-02")
	for _, ch := range future {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab through start/stop/tag/note.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// start is 09:00, Tab to stop
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for _, ch := range "+1h" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	out := m.View().Content
	if !strings.Contains(strings.ToLower(out), "zukunft") {
		t.Errorf("future date should show error about Zukunft, got:\n%s", out)
	}
}

func TestListAdd_MondayShowsFridayDate(t *testing.T) {
	r := newRig(t)
	r.clock.T = time.Date(2026, 5, 4, 10, 0, 0, 0, time.Local) // Monday
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	friday := "2026-05-01"
	if !strings.Contains(out, friday) {
		t.Errorf("on Monday, list add should seed Friday (%s), got:\n%s", friday, out)
	}
	_ = domain.Session{}
}
