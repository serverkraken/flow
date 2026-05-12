package worktime_test

// History drill attached-notes visibility + inline viewer tests.
// Round-5b adds the chip line + `o`-key viewer so users can actually
// see what they attached via the round-5a `n`-picker.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestHistory_DrillAttached_ChipShowsAfterAttach pinst die Sichtbarkeit:
// nach erfolgreichem n-Attach lädt der Drill nach (historyActionDoneMsg
// → drillLoadCmd), und die Chip-Zeile muss die ID enthalten.
func TestHistory_DrillAttached_ChipShowsAfterAttach(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Open drill on the focused row.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Attach a note.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	id := "daily/2026-04-28"
	for _, ch := range id {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if !strings.Contains(out, id) {
		t.Errorf("Drill should show attached note ID %q in chip line, got:\n%s", id, out)
	}
	// "o → ansehen" hint should appear in the drill footer since we
	// now have ≥1 attached note.
	if !strings.Contains(out, "o → ansehen") {
		t.Errorf("Drill footer should advertise o-key viewer when attached present, got:\n%s", out)
	}
}

// TestHistory_DrillAttached_LoadedOnOpen pinst dass attached IDs schon
// beim ersten Drill-Open geladen werden (nicht erst nach einer
// Attach-Aktion). User-Story: ein Tag wurde vorher mal angehängt; beim
// Drill müssen die IDs sofort sichtbar sein.
func TestHistory_DrillAttached_LoadedOnOpen(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	// Pre-seed an attachment on the most recent session day. The
	// list-default cursor sits on the newest record; seedHistorySessions
	// puts the latest entry on isoMondayOf(now).AddDate(0,0,2) =
	// Wednesday of the current ISO week.
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "daily/preseeded"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	// Cursor is 0 on the most recent record; verify which date Enter drills into.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if !strings.Contains(out, preID) {
		t.Logf("note: drill cursor may not have hit the pre-seeded date — view was:\n%s", out)
		// Soft-fail: at least one of the seeded dates should drill to wed.
		// Try j (next) until we find it or wrap around — but with the
		// list ordering being newest-first and seedHistorySessions
		// providing 3 same-week rows + 1 prev-week, the cursor 0 in this
		// rig hits the most-recent row which is wed. If the assertion
		// fails it's a real ordering regression worth surfacing.
		t.Errorf("attached note %q should be visible immediately on drill open, got:\n%s", preID, out)
	}
}

// TestHistory_DrillO_OpensInlineViewer pinst den `o`-Pfad: Overlay
// erscheint, die Note-ID steht im Title-Strip des markdown_overlay.
func TestHistory_DrillO_OpensInlineViewer(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "notes/some-note"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Press o — opens markdown_overlay.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	out := m.View()
	// markdown_overlay renders title via "Note · <id>". Search for the
	// stable suffix (the · separator + ID).
	if !strings.Contains(out, "Note · "+preID) {
		t.Errorf("o-key should open markdown_overlay with Note title, got:\n%s", out)
	}
	// Esc closes the viewer — markdown_overlay emits ExitMsg via cmd,
	// drain so the worktime root sees it and routes back into history.
	m, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = drainCmd(t, m, cmd2)
	out = m.View()
	if strings.Contains(out, "Note · "+preID) {
		t.Errorf("Esc should close the viewer, got:\n%s", out)
	}
	if !strings.Contains(out, preID) {
		t.Errorf("after Esc the drill chip line should still show %q, got:\n%s", preID, out)
	}
}

// TestHistory_DrillO_NoAttached_ShowsToast pinst den Degenerationspfad:
// `o` ohne angehängte Notes → Info-Toast, kein Overlay-Open.
func TestHistory_DrillO_NoAttached_ShowsToast(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// No attach happened — press o.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if strings.Contains(out, "Note ·") {
		t.Errorf("o-key without attached should NOT open viewer, got:\n%s", out)
	}
	if !strings.Contains(out, "Keine Notiz") {
		t.Errorf("o-key without attached should toast »Keine Notiz«, got:\n%s", out)
	}
}

// TestHistory_DrillCapO_OpensExternalEditor pinst den `O`-Pfad: ruft
// deps.NoteOpener.Open(id) genau einmal mit der drillDate-ID auf.
func TestHistory_DrillCapO_OpensExternalEditor(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "notes/external-edit"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Press O (uppercase) — fires NoteOpener.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("O")})
	_ = drainCmd(t, m, cmd)
	if len(r.noteLauncher.Calls) != 1 {
		t.Fatalf("NoteOpener should be called once, got %d calls: %+v",
			len(r.noteLauncher.Calls), r.noteLauncher.Calls)
	}
	if want := "open:" + preID; r.noteLauncher.Calls[0] != want {
		t.Errorf("NoteOpener call = %q, want %q", r.noteLauncher.Calls[0], want)
	}
}

// TestHistory_DrillR_DetachesFirstAttachedNote pinst den `R`-Pfad:
// LinkWriter.Remove(drillDate, firstID) wird gefeuert, der Chip
// verschwindet im naechsten Render (drillLoadCmd zieht nach).
func TestHistory_DrillR_DetachesFirstAttachedNote(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "notes/to-detach"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Chip ist sichtbar.
	if !strings.Contains(m.View(), preID) {
		t.Fatalf("pre-condition: chip should show preID, got:\n%s", m.View())
	}
	// Press R — detach.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = drainCmd(t, m, cmd)
	// LinkStore leer für wed.
	if ids := r.links.ByDate[wed.Format("2006-01-02")]; len(ids) != 0 {
		t.Errorf("LinkStore should be empty for drillDate after R, got: %+v", ids)
	}
	// Chip-Marker (●) verschwindet — die preID ist u.U. noch im Toast,
	// deshalb matchen wir den Chip-spezifischen Prefix statt nur die ID.
	if strings.Contains(m.View(), "●  "+preID) {
		t.Errorf("chip line should disappear after R, got:\n%s", m.View())
	}
	// Toast erscheint kurz im drillToast-Slot.
	if !strings.Contains(m.View(), "entfernt") {
		t.Errorf("toast should confirm »entfernt«, got:\n%s", m.View())
	}
}

// TestHistory_DrillR_NoAttached_ShowsToast pinst den Degenerationspfad:
// `R` ohne angehängte Notes → Info-Toast, kein LinkWriter-Call.
func TestHistory_DrillR_NoAttached_ShowsToast(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = drainCmd(t, m, cmd)
	if !strings.Contains(m.View(), "Keine Notiz") {
		t.Errorf("R without attached should toast »Keine Notiz«, got:\n%s", m.View())
	}
}
