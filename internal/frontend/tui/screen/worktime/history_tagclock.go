package worktime

// History tag-clock mode — fixed 24×7 grid (hour × weekday) showing
// where the user typically works. Aggregates per-(weekday, hour)
// duration across the filtered records and shades each cell relative
// to the largest cell (so the contrast survives sparse data sets).

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
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

func tagClockCellGlyph(pal theme.Palette, cell time.Duration, frac float64) (string, lipgloss.TerminalColor) {
	switch {
	case cell == 0:
		return "··", pal.BgCode
	case frac >= 0.75:
		return "██", pal.Green
	case frac >= 0.5:
		return "▓▓", pal.Green
	case frac >= 0.25:
		return "▒▒", pal.Yellow
	case frac > 0:
		return "░░", pal.Yellow
	}
	return "··", pal.BgCode
}

func (h history) renderTagClockHeader() string {
	hdr := "      "
	for col := 0; col < 24; col++ {
		c := h.pal.FgMuted
		if col == 9 || col == 12 || col == 17 {
			c = h.pal.BgCode
		}
		hdr += lipgloss.NewStyle().Foreground(c).Render(fmt.Sprintf("%02d", col))
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
				cellStyle = lipgloss.NewStyle().Foreground(h.pal.Bg).Background(h.pal.Sem().Accent).Bold(true)
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
	legend := []string{
		stDim(h.pal, "·· keine"),
		stDim(h.pal, "░░ <25%"),
		stDim(h.pal, "▒▒ <50%"),
		stDim(h.pal, "▓▓ <75%"),
		stDim(h.pal, "██ ≥75%"),
	}
	lines = append(lines, joinWrapped(legend, "  ", "   ", "   ", inner))
	return strings.Join(lines, "\n")
}
