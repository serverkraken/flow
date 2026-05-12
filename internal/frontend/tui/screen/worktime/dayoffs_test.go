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
