package worktime_test

// History drill attached-notes visibility + inline viewer tests.
// Round-5b adds the chip line + `o`-key viewer so users can actually
// see what they attached via the round-5a `n`-picker.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestHistory_DrillAttached_ChipShowsAfterAttach pinst die Sichtbarkeit:
// nach erfolgreichem n-Attach lädt der Drill nach (historyActionDoneMsg
// → drillLoadCmd), und die Chip-Zeile muss die ID enthalten.
func TestHistory_DrillAttached_ChipShowsAfterAttach(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// Open drill on the focused row.
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Attach a note.
	m, _ = m.Update(tea.KeyPressMsg{Text: "n"})
	id := "daily/2026-04-28"
	for _, ch := range id {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Press o — opens markdown_overlay.
	m, _ = m.Update(tea.KeyPressMsg{Text: "o"})
	out := m.View()
	// markdown_overlay renders title via "Note · <id>". Search for the
	// stable suffix (the · separator + ID).
	if !strings.Contains(out, "Note · "+preID) {
		t.Errorf("o-key should open markdown_overlay with Note title, got:\n%s", out)
	}
	// Esc closes the viewer — markdown_overlay emits ExitMsg via cmd,
	// drain so the worktime root sees it and routes back into history.
	m, cmd2 := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// No attach happened — press o.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "o"})
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
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Press O (uppercase) — fires NoteOpener.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "O"})
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
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// Chip ist sichtbar.
	if !strings.Contains(m.View(), preID) {
		t.Fatalf("pre-condition: chip should show preID, got:\n%s", m.View())
	}
	// Press R — detach.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "R"})
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

// TestHistory_List_ShowsAttachedNotesMarker pinst den ●-Marker pro Zeile
// im List-Mode: Tage mit Notes kriegen einen ● (1 Note) bzw. ● N (>1)
// Suffix; Tage ohne Notes bleiben unmarkiert. Damit sieht der User
// schon vor dem Drilldown, dass an einem Tag Notizen haengen.
func TestHistory_List_ShowsAttachedNotesMarker(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	tue := mon.AddDate(0, 0, 1)
	wed := mon.AddDate(0, 0, 2)
	if err := r.links.Add(mon, "notes/one"); err != nil {
		t.Fatalf("seed mon: %v", err)
	}
	if err := r.links.Add(wed, "notes/a"); err != nil {
		t.Fatalf("seed wed-a: %v", err)
	}
	if err := r.links.Add(wed, "notes/b"); err != nil {
		t.Fatalf("seed wed-b: %v", err)
	}
	m := loadedHistory(t, r)
	out := m.View()
	// Mon: 1 Note → ● (kein Count).
	monLine := lineContaining(t, out, mon.Format("02.01.06"))
	if !strings.Contains(monLine, "●") || strings.Contains(monLine, "● 2") {
		t.Errorf("Montag (1 Note) sollte einzelnes ● zeigen, kein Count: %q", monLine)
	}
	// Wed: 2 Notes → ● 2.
	wedLine := lineContaining(t, out, wed.Format("02.01.06"))
	if !strings.Contains(wedLine, "● 2") {
		t.Errorf("Mittwoch (2 Notes) sollte »● 2« zeigen: %q", wedLine)
	}
	// Tue: 0 Notes → kein Marker.
	tueLine := lineContaining(t, out, tue.Format("02.01.06"))
	if strings.Contains(tueLine, "●") {
		t.Errorf("Dienstag (0 Notes) darf keinen ●-Marker zeigen: %q", tueLine)
	}
}

// TestHistory_Heatmap_StatusShowsAttachedNotesMarker pinst dass der
// Heatmap-Cursor-Status den ●-Marker zeigt, wenn der fokussierte Tag
// Notes hat. Heatmap-Cells selbst sind 3-char-glyph-eng, ein In-Cell-
// Marker wuerde mit der Heat-Glyph-Semantik kollidieren — der Status
// unter dem Grid ist die richtige Surface.
func TestHistory_Heatmap_StatusShowsAttachedNotesMarker(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	if err := r.links.Add(wed, "notes/x"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.links.Add(wed, "notes/y"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	// `v` von Liste → Heatmap.
	m, _ = m.Update(tea.KeyPressMsg{Text: "v"})
	// Cursor steht initial auf der jüngsten Session (wed) — siehe
	// handleListKey "v" branch der den heatCol/heatRow vorbelegt.
	out := m.View()
	if !strings.Contains(out, "● 2") {
		t.Errorf("Heatmap-Status sollte »● 2« zeigen, got:\n%s", out)
	}
}

// TestHistory_Month_StatusShowsAttachedNotesMarker spiegelt den
// Heatmap-Test fuer das Monatsraster: Cursor-Status haengt den ●-Marker
// an, wenn der fokussierte Tag Notes hat.
func TestHistory_Month_StatusShowsAttachedNotesMarker(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	// Cursor landet beim Mode-Wechsel auf records[0].Date — die juengste
	// Session, hier Wed (mon+2) per seedHistorySessions. Link an genau
	// diesen Tag haengen, damit der Cursor-Status den Marker triggert.
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	if err := r.links.Add(wed, "notes/wed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)
	// Liste → Heatmap → TagClock → Monat (3× v).
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Text: "v"})
	}
	out := m.View()
	if !strings.Contains(out, "●") {
		t.Errorf("Monats-Cursor-Status sollte ●-Marker zeigen wenn Notes da sind, got:\n%s", out)
	}
}

// lineContaining sucht die erste Zeile in s, die marker enthaelt.
// Helper fuer Per-Row-Assertions im List-Mode wo strings.Contains auf
// dem ganzen Output zu grob waere.
func lineContaining(t *testing.T, s, marker string) string {
	t.Helper()
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("keine Zeile enthaelt %q in:\n%s", marker, s)
	return ""
}

// TestHistory_DrillR_NoAttached_ShowsToast pinst den Degenerationspfad:
// `R` ohne angehängte Notes → Info-Toast, kein LinkWriter-Call.
func TestHistory_DrillR_NoAttached_ShowsToast(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Text: "R"})
	m = drainCmd(t, m, cmd)
	if !strings.Contains(m.View(), "Keine Notiz") {
		t.Errorf("R without attached should toast »Keine Notiz«, got:\n%s", m.View())
	}
}
