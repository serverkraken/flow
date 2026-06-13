package worktime

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestRenderKindSummary_LabelsInKindColor: Übersichts-Chips oben im
// Frei-Tab zeigen jedes Kind-Label in seiner Sem-Farbe (statt pauschal
// dim). Der Count bleibt dim, damit die Hierarchie Label > Count sichtbar
// bleibt. Spec: 2026-05-12-unified-dayoff-glyphs.
func TestRenderKindSummary_LabelsInKindColor(t *testing.T) {
	pal := theme.TokyonightNight
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	f := frei{
		pal: pal,
		entries: []domain.DayOff{
			{Date: may, Kind: domain.KindHoliday, Label: "H1"},
			{Date: may.AddDate(0, 0, 1), Kind: domain.KindHoliday, Label: "H2"},
			{Date: may.AddDate(0, 0, 2), Kind: domain.KindVacation, Label: "U"},
			{Date: may.AddDate(0, 0, 3), Kind: domain.KindSick, Label: "K"},
		},
	}
	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}
	out := f.renderKindSummary()
	tests := []struct {
		label string
		color string
	}{
		{"Feiertag", colorSeq(pal.Blue)},
		{"Urlaub", colorSeq(pal.Purple)},
		{"Krank", colorSeq(pal.Orange)},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			if !strings.Contains(out, tc.label) {
				t.Errorf("summary missing label %q: %q", tc.label, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("summary missing colour %q for label %q: %q", tc.color, tc.label, out)
			}
		})
	}
}

// TestRenderKindPicker_LeadingColoredGlyph: jeder Kind-Chip im Add-Dialog
// trägt einen führenden ○ in der Kind-Farbe, auch wenn das Chip nicht
// selektiert ist. Selektierter Chip behält Accent (One-Accent-Per-Row).
// Spec: 2026-05-12-unified-dayoff-glyphs.
func TestRenderKindPicker_LeadingColoredGlyph(t *testing.T) {
	pal := theme.TokyonightNight
	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}
	// kindCur=-1 (out of range) so NO chip is selected and all three
	// carry their Kind-Farbe. This allows asserting all three Sem-colours.
	f := frei{pal: pal, dialog: freiDialogAdd, kindCur: -1, formCur: 0}
	out := f.renderKindPicker(80)

	// The glyph and label are rendered as separate lipgloss spans, so we
	// check each component independently: colour sequence + label text.
	// Spec 2026-05-13: ● glyph (Filled) for every kind chip, cross-surface
	// with tmux pace dots.
	glyphCount := strings.Count(out, "●")
	if glyphCount < 3 {
		t.Errorf("picker: want >=3 ● glyphs, got %d: %q", glyphCount, out)
	}
	tests := []struct {
		label string
		color string
	}{
		{"Feiertag", colorSeq(pal.Blue)},
		{"Urlaub", colorSeq(pal.Purple)},
		{"Krank", colorSeq(pal.Orange)},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			if !strings.Contains(out, tc.label) {
				t.Errorf("picker missing label %q: %q", tc.label, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("picker missing colour %q for %q: %q", tc.color, tc.label, out)
			}
		})
	}
}

// TestRenderEntryRow_UserLabelInKindColor: jeder Entry-Row im Frei-Tab
// rendert das User-Label (z.B. "Tag der Arbeit") in der Kind-Farbe — der
// Kind-Spalte gleichgesetzt, damit der ganze Zeilen-Identitätsteil
// einfarbig liest. Datum bleibt FgMuted als kontextuelles Präfix.
// Spec: 2026-05-13-filled-dayoff-dots-supersede.
func TestRenderEntryRow_UserLabelInKindColor(t *testing.T) {
	pal := theme.TokyonightNight
	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	tests := []struct {
		kind  domain.Kind
		label string
		color string
	}{
		{domain.KindHoliday, "Tag der Arbeit", colorSeq(pal.Blue)},
		{domain.KindVacation, "Brückentag", colorSeq(pal.Purple)},
		{domain.KindSick, "Grippe", colorSeq(pal.Orange)},
	}
	// Cursor weg vom 0. Eintrag, damit die Selektions-Accent-Farbe nicht
	// die Per-Kind-Farb-Erwartung überschreibt.
	f := frei{pal: pal, cursor: 99, width: 100}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			out := f.renderEntryRow(0, domain.DayOff{
				Date: may, Kind: tc.kind, Label: tc.label,
			}, 80)
			if !strings.Contains(out, tc.label) {
				t.Errorf("row missing label %q: %q", tc.label, out)
			}
			// User-Label in Kind-Farbe: das Label-Wort muss vom Kind-
			// Color-Sequence eingerahmt sein. Eine einfache Substring-
			// Prüfung "<color><…>label" ist ausreichend, weil lipgloss
			// jeden Foreground-Span eigenständig schließt.
			if !strings.Contains(out, tc.color+"m"+tc.label) {
				t.Errorf("row should colour user label %q with %q, got: %q", tc.label, tc.color, out)
			}
		})
	}
}

// TestFrei_FooterHints_IncludesHelp mirrors week.TestFooterHints_ContainsHelp
// for the Frei sub-tab: Skill §Keybind grammar pins `?` as a fixed-slot
// key that must be discoverable from every screen footer. Phase-10
// follow-up to the 2026-05-30 UX-Review-Cleanup; the `:`-actions hint
// moved to the `?`-overlay (action menu still reachable via Palette
// and ?-overlay) to make room for HintHelp inside the 4-cap.
func TestFrei_FooterHints_IncludesHelp(t *testing.T) {
	f := frei{pal: theme.TokyonightNight}
	hints := f.footerHints()
	found := false
	for _, x := range hints {
		if strings.Contains(x, "? → Hilfe") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Frei footerHints: expected ?-help hint, got %v", hints)
	}
}

// TestFrei_DeleteUsesCapturedDate verifies that the delete action uses the
// date captured when the confirm dialog opens, NOT the date at the cursor
// index at the time the user confirms. A reload that reorders entries while
// the dialog is open must not cause the wrong entry to be deleted.
func TestFrei_DeleteUsesCapturedDate(t *testing.T) {
	pal := theme.TokyonightNight

	// Three entries sorted chronologically so List() returns them in order:
	// cursor 0 → D1 (2026-06-01), cursor 1 → D2 (2026-06-02), cursor 2 → D3 (2026-06-03)
	d1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	d2 := time.Date(2026, 6, 2, 0, 0, 0, 0, time.Local)
	d3 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.Local)

	store := testutil.NewFakeDayOffStore(
		domain.DayOff{Date: d1, Kind: domain.KindVacation, Label: "Eintrag1"},
		domain.DayOff{Date: d2, Kind: domain.KindVacation, Label: "Eintrag2"},
		domain.DayOff{Date: d3, Kind: domain.KindVacation, Label: "Eintrag3"},
	)
	writer := &usecase.DayOffWriter{Store: store}

	// Build frei with cursor at index 1 (D2) and entries pre-seeded.
	f := frei{
		pal:    pal,
		deps:   Deps{DayOffWriter: writer, DayOffStore: store},
		year:   2026,
		loaded: true,
		cursor: 1, // points at D2
		entries: []domain.DayOff{
			{Date: d1, Kind: domain.KindVacation, Label: "Eintrag1"},
			{Date: d2, Kind: domain.KindVacation, Label: "Eintrag2"},
			{Date: d3, Kind: domain.KindVacation, Label: "Eintrag3"},
		},
	}

	// Step 1: press D — opens confirm for D2 (cursor=1).
	var m tea.Model = f
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Text: "D"})
	_ = cmd // Init cmd from confirmModel — not needed for the test

	// Step 2: inject a reload that shuffles entries so cursor 1 now points
	// at D3 (D1 and D2 swapped out, D3 moved to index 1).
	reloaded := freiLoadedMsg{
		year: 2026,
		entries: []domain.DayOff{
			{Date: d3, Kind: domain.KindVacation, Label: "Eintrag3"},
			{Date: d1, Kind: domain.KindVacation, Label: "Eintrag1"},
			{Date: d2, Kind: domain.KindVacation, Label: "Eintrag2"},
		},
	}
	m, cmd = m.Update(reloaded)
	_ = cmd

	// Step 3: confirm the deletion — send confirm.ResultMsg{Confirmed: true}
	// directly (as the confirm.Model would emit via its cmd).
	m, cmd = m.Update(confirm.ResultMsg{Confirmed: true})
	// Drain the cmd (the mutation + emitWorktimeChanged batch).
	if cmd != nil {
		type batchMsg = []tea.Cmd
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					_ = c()
				}
			}
		}
	}
	_ = m

	// Assert: D2 (the originally confirmed date) must be gone from the store.
	if _, ok := store.Entries[d2.Format("2006-01-02")]; ok {
		t.Errorf("expected D2 (%s) to be deleted, but it is still in the store", d2.Format("2006-01-02"))
	}
	// D1 and D3 must NOT have been deleted.
	if _, ok := store.Entries[d1.Format("2006-01-02")]; !ok {
		t.Errorf("D1 (%s) was unexpectedly deleted", d1.Format("2006-01-02"))
	}
	if _, ok := store.Entries[d3.Format("2006-01-02")]; !ok {
		t.Errorf("D3 (%s) was unexpectedly deleted", d3.Format("2006-01-02"))
	}
}
