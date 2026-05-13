package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
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
	colorSeq := func(c lipgloss.Color) string {
		return termenv.RGBColor(string(c)).Sequence(false)
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
	colorSeq := func(c lipgloss.Color) string {
		return termenv.RGBColor(string(c)).Sequence(false)
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
	colorSeq := func(c lipgloss.Color) string {
		return termenv.RGBColor(string(c)).Sequence(false)
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
