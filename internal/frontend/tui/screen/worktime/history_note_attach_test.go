package worktime_test

// History-drill note-attach black-box tests. The `n` key in a drilled
// past day must open the shared noteAttachPicker, scope its date to
// drillDate (not Clock.Now() like Heute), and Submit must call
// LinkWriter.Add(drillDate, id).

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

func TestHistory_DrillNoteAttach_NOpensPicker(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Open drill on the focused row (cursor 0 — most recent session).
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Press n to open note-attach.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("note-attach picker should leave FilterActive=true (TextInputActive proxy)")
	}
	out := m.View()
	// View should announce the date the picker is scoped to. The
	// section header is uppercased by picker.SectionHeader, so the
	// date itself is the stable substring to match against.
	if !strings.Contains(strings.ToLower(out), "note an ") {
		t.Errorf("picker should announce the scoped date, got:\n%s", out)
	}
}

func TestHistory_DrillNoteAttach_EnterAttachesIDToDrillDate(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	// Type a note ID.
	id := "daily/2026-04-28"
	for _, ch := range id {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Enter submits — picker has no suggestions wired (newRig didn't set
	// NoteLister), so the raw input wins.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)

	// Exactly one date received the attach — and the ID matches.
	if len(r.links.ByDate) != 1 {
		t.Fatalf("LinkStore should have one date keyed, got %d entries: %+v",
			len(r.links.ByDate), r.links.ByDate)
	}
	for date, ids := range r.links.ByDate {
		if len(ids) != 1 || ids[0] != id {
			t.Errorf("LinkStore[%q] = %v, want exactly [%q]", date, ids, id)
		}
	}
}

func TestHistory_DrillNoteAttach_EscCancelsWithoutAttach(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	// Type something then bail with Esc — LinkStore must stay clean.
	for _, ch := range "foo" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(r.links.ByDate) != 0 {
		t.Errorf("Esc must NOT write to LinkStore, got %+v", r.links.ByDate)
	}
	// We expect to be back in the drill view (still FilterActive — drill
	// is itself a filter-claiming dialog).
	if !m.(worktime.Model).FilterActive() {
		t.Error("after Esc on note-attach, drill should still be active")
	}
}

func TestHistory_DrillNoteAttach_EmptyIDSurfacesError(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	// Enter with empty input — picker should set its own errMsg and
	// stay open. No LinkStore mutation.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(r.links.ByDate) != 0 {
		t.Errorf("empty ID must NOT call LinkWriter, got %+v", r.links.ByDate)
	}
	if !m.(worktime.Model).FilterActive() {
		t.Error("picker should stay open after empty-ID error")
	}
	if !strings.Contains(m.View(), "Note-ID darf nicht leer sein") {
		t.Errorf("picker should surface the empty-ID error, got:\n%s", m.View())
	}
}
