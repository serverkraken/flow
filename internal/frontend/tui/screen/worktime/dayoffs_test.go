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
	sem := pal.Sem()
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
		{"Feiertag", colorSeq(sem.Info)},
		{"Urlaub", colorSeq(sem.Success)},
		{"Krank", colorSeq(sem.Warning)},
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
	sem := pal.Sem()
	colorSeq := func(c lipgloss.Color) string {
		return termenv.RGBColor(string(c)).Sequence(false)
	}
	// kindCur=-1 (out of range) so NO chip is selected and all three
	// carry their Kind-Farbe. This allows asserting all three Sem-colours.
	f := frei{pal: pal, dialog: freiDialogAdd, kindCur: -1, formCur: 0}
	out := f.renderKindPicker(80)

	// The glyph and label are rendered as separate lipgloss spans, so we
	// check each component independently: colour sequence + label text.
	// The ○ glyph must appear at least once per kind (three total).
	glyphCount := strings.Count(out, "○")
	if glyphCount < 3 {
		t.Errorf("picker: want >=3 ○ glyphs, got %d: %q", glyphCount, out)
	}
	tests := []struct {
		label string
		color string
	}{
		{"Feiertag", colorSeq(sem.Info)},
		{"Urlaub", colorSeq(sem.Success)},
		{"Krank", colorSeq(sem.Warning)},
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
