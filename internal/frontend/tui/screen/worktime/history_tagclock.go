package worktime

// History tag-clock mode — fixed 24×7 grid (hour × weekday) showing
// where the user typically works. Aggregates per-(weekday, hour)
// duration across the filtered records and shades each cell relative
// to the largest cell (so the contrast survives sparse data sets).

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// tagClockGrid sums per-(weekday, hour) durations across the records'
// sessions, splitting on hour boundaries. Returns the grid and the
// largest single cell so callers can scale fractions.
func tagClockGrid(records []domain.DayRecord) ([7][24]time.Duration, time.Duration) {
	var grid [7][24]time.Duration
	for _, rec := range records {
		for _, s := range rec.Sessions {
			t := s.Start
			for t.Before(s.Stop) {
				wd := int(t.Weekday()) - 1
				if wd < 0 {
					wd = 6
				}
				hour := t.Hour()
				next := time.Date(t.Year(), t.Month(), t.Day(), hour+1, 0, 0, 0, t.Location())
				if next.After(s.Stop) {
					next = s.Stop
				}
				grid[wd][hour] += next.Sub(t)
				t = next
			}
		}
	}
	var maxCell time.Duration
	for r := 0; r < 7; r++ {
		for c := 0; c < 24; c++ {
			if grid[r][c] > maxCell {
				maxCell = grid[r][c]
			}
		}
	}
	return grid, maxCell
}

// tagClockStep ties one density bucket to its glyph, legend label and
// color. The cell renderer and the legend both iterate tagClockScale,
// so glyph + Farbe je Bucket können nicht auseinanderdriften. Die Zelle
// rendert den Glyph doppelt (zwei Zellen breit). Reihenfolge high→low.
type tagClockStep struct {
	minFrac float64
	glyph   string
	label   string
	color   func(theme.Palette) color.Color
}

var tagClockScale = []tagClockStep{
	{0.75, glyphs.HeatFull, "≥75%", func(p theme.Palette) color.Color { return p.Sem().Success }},
	{0.5, glyphs.HeatDark, "<75%", func(p theme.Palette) color.Color { return p.Sem().Active }},
	{0.25, glyphs.HeatMedium, "<50%", func(p theme.Palette) color.Color { return p.Sem().Warning }},
	{0, glyphs.HeatLight, "<25%", func(p theme.Palette) color.Color { return p.FgMuted }},
}

func tagClockCellGlyph(pal theme.Palette, cell time.Duration, frac float64) (string, color.Color) {
	if cell == 0 {
		return strings.Repeat(glyphs.BulletDot, 2), pal.BgCode
	}
	for _, s := range tagClockScale {
		if frac >= s.minFrac {
			return strings.Repeat(s.glyph, 2), s.color(pal)
		}
	}
	return strings.Repeat(glyphs.BulletDot, 2), pal.BgCode
}

func (h history) renderTagClockHeader() string {
	hdr := "      "
	for col := 0; col < 24; col++ {
		// Landmark-Stunden (9/12/17) bold-Cyan damit Morning/Noon/EOD-
		// Boundaries als Orientierungspunkte erkennbar bleiben — vorher
		// kollidierten sie als BgCode mit der Hintergrundfarbe (A11y-2).
		c := h.pal.FgMuted
		bold := false
		if col == 9 || col == 12 || col == 17 {
			c = h.pal.Sem().Info
			bold = true
		}
		hdr += lipgloss.NewStyle().Foreground(c).Bold(bold).Render(fmt.Sprintf("%02d", col))
	}
	return hdr
}

func (h history) renderTagClockRows(grid [7][24]time.Duration, maxCell time.Duration, dayLabels []string) []string {
	out := make([]string, 0, 7)
	for r := 0; r < 7; r++ {
		row := "  " + lipgloss.NewStyle().Foreground(h.pal.Fg).Width(3).Render(dayLabels[r]) + " "
		for c := 0; c < 24; c++ {
			frac := float64(grid[r][c]) / float64(maxCell)
			cell, color := tagClockCellGlyph(h.pal, grid[r][c], frac)
			cellStyle := lipgloss.NewStyle().Foreground(color)
			if r == h.tagClockRow && c == h.tagClockCol {
				cellStyle = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Sem().Accent).Bold(true).Underline(true)
			}
			row += cellStyle.Render(cell)
		}
		out = append(out, row)
	}
	return out
}

func (h history) renderTagClockStatus(grid [7][24]time.Duration, maxCell time.Duration, dayLabels []string) string {
	col, row := h.tagClockCol, h.tagClockRow
	if row < 0 || row >= 7 || col < 0 || col >= 24 {
		return ""
	}
	dur := grid[row][col]
	var status string
	if dur == 0 {
		status = fmt.Sprintf("   %s  %02d:00–%02d:00  —",
			dayLabels[row], col, (col+1)%24)
	} else {
		pct := int(float64(dur) / float64(maxCell) * 100)
		status = fmt.Sprintf("   %s  %02d:00–%02d:00  %s  (%d%% des Maximums)",
			dayLabels[row], col, (col+1)%24, formatDur(dur), pct)
	}
	return lipgloss.NewStyle().Foreground(h.pal.Sem().Accent).Render(status)
}

func (h history) renderTagClock(records []domain.DayRecord, inner int) string {
	if len(records) == 0 {
		return stDim(h.pal, "  Keine Treffer.")
	}
	// Tag-Clock-Grid ist fix 24 Spalten × 2 Char + 6 Char Prefix = 54
	// Char Mindestbreite. Schmalere Panes würden die Spalten am
	// rechten Rand abscheren (titlebox-Truncation) — besser ein klarer
	// Hinweis als ein halbes Grid.
	if inner < 54 {
		return stDim(h.pal, "  Pane zu schmal für Tag-Clock — `v` schaltet Ansicht um.")
	}
	grid, maxCell := tagClockGrid(records)
	if maxCell == 0 {
		return stDim(h.pal, "  Keine Treffer.")
	}
	dayLabels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	lines := []string{h.renderTagClockHeader()}
	lines = append(lines, h.renderTagClockRows(grid, maxCell, dayLabels)...)
	lines = append(lines, "")
	if status := h.renderTagClockStatus(grid, maxCell, dayLabels); status != "" {
		lines = append(lines, status)
	}
	legend := []string{stDim(h.pal, strings.Repeat(glyphs.BulletDot, 2)+" keine")}
	for i := len(tagClockScale) - 1; i >= 0; i-- {
		s := tagClockScale[i]
		legend = append(legend,
			lipgloss.NewStyle().Foreground(s.color(h.pal)).Render(strings.Repeat(s.glyph, 2)+" "+s.label))
	}
	lines = append(lines, joinWrapped(legend, "  ", "   ", "   ", inner))
	return strings.Join(lines, "\n")
}
