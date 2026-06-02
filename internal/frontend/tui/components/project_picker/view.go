package project_picker

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// pickerTitle is the picker's title string. German per §German UI conventions.
const pickerTitle = "Projekt wählen"

// newRowLabel is the sticky bottom entry that triggers inline project creation.
const newRowLabel = "+ Neues Projekt anlegen"

// View renders the picker. Returns "" when the terminal dimensions are
// not set or too small.
func (m Model) View() string {
	if m.width < 10 || m.height < 6 {
		return ""
	}
	return m.renderContent()
}

// renderContent assembles the full picker: titlebox + body + footer hint.
// The titlebox chrome (frame + title) takes the outer width; the body
// fills the remaining lines.
func (m Model) renderContent() string {
	s := styles()
	// Effective inner width for row content: subtract the frame
	// border (2) and padding (PadXS * 2 = 2) from the total width.
	// Under lipgloss v2, Style.Width is outer-total so we pass m.width
	// directly to titlebox; the inner content width is m.width - 4.
	innerW := m.width - 4
	if innerW < 4 {
		innerW = 4
	}

	// Filter line: "▶ <filter or placeholder>"
	filterLine := m.renderFilterLine(innerW)

	// Separator rule.
	sep := s.separator.Render(strings.Repeat("─", innerW))

	// List rows: visible items + sticky "+Neu" entry.
	listRows := m.renderList(innerW)

	body := filterLine + "\n" + sep + "\n" + strings.Join(listRows, "\n")

	box := titlebox.Render(pickerTitle, body, m.width, m.palette)

	// Footer hints (≤4 entries per §Hint format).
	hints := strings.Join([]string{
		"↑/↓ → navigieren",
		"tab → Neu anlegen",
		"Enter → wählen",
		uistrings.HintCancel,
	}, "  ·  ")
	footer := s.footer.Render(hints)

	return box + "\n" + footer
}

// renderFilterLine renders the filter input row. The ▶ (Active) glyph
// signals that the picker always has filter focus — there is no "unfocused"
// state because the picker owns the full screen. FgMuted placeholder when
// the filter is empty.
func (m Model) renderFilterLine(innerW int) string {
	s := styles()
	pfx := s.filterPfx.Render(glyphs.Active + " ")
	content := m.filter
	if content == "" {
		content = theme.Dim("filtern…", m.palette)
	}
	// Truncate the filter display if it would overflow the row.
	maxFilterW := innerW - lipgloss.Width(pfx) - 1
	if maxFilterW < 1 {
		maxFilterW = 1
	}
	content = uistrings.Truncate(content, maxFilterW)
	return pfx + content
}

// renderList builds the slice of rendered row strings for the list body.
// It includes the filtered items (with fuzzy-match highlights) followed by
// the sticky "+ Neues Projekt anlegen" entry.
func (m Model) renderList(innerW int) []string {
	s := styles()
	maxVis := m.maxVisible()

	var rows []string

	if len(m.filtered) == 0 && m.filter != "" {
		// No matches — show empty-state hint but still render "+Neu" below.
		rows = append(rows, s.noMatches.Render("  "+uistrings.LabelEmpty))
	} else {
		// Scroll window: clamp offset so cursor stays in view.
		offset := m.scrollOffset(maxVis)
		end := offset + maxVis
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		if offset > 0 {
			rows = append(rows, theme.Dim("  "+glyphs.Up+" "+itoa(offset)+" vorherige…", m.palette))
		}

		for i := offset; i < end; i++ {
			selected := (m.cursor == i)
			var hi []int
			if i < len(m.highlights) {
				hi = m.highlights[i]
			}
			rows = append(rows, picker.RowWithMatch(picker.RowWithMatchOpts{
				Selected: selected,
				Label:    m.filtered[i].Name,
				Width:    innerW,
				Match:    hi,
			}, m.palette))
		}

		remaining := len(m.filtered) - end
		if remaining > 0 {
			rows = append(rows, theme.Dim("  "+glyphs.Down+" "+itoa(remaining)+" weitere…", m.palette))
		}
	}

	// Sticky "+Neu" entry — always last, never hidden by filter.
	neuSel := m.cursor == m.neuIdx()
	rows = append(rows, renderNewRow(neuSel, innerW, s))

	return rows
}

// renderNewRow renders the "+ Neues Projekt anlegen" pseudo-row.
// When selected it shows the accent bar (▎) and bold green label;
// unselected shows a plain space prefix and normal-weight green label.
func renderNewRow(selected bool, width int, s *pickerStyles) string {
	bar := " "
	label := newRowLabel
	if selected {
		bar = s.accentBar.Render(glyphs.AccentBar)
		label = s.newRowSel.Render(label)
	} else {
		label = s.newRow.Render(label)
	}
	// Pad to inner width so the row aligns with the others.
	labelW := lipgloss.Width(label)
	gap := width - 1 - labelW - 1
	if gap < 0 {
		gap = 0
	}
	return bar + " " + label + strings.Repeat(" ", gap)
}

// maxVisible returns the number of list rows that fit within the current
// terminal height, accounting for chrome (frame borders + filter + separator
// + footer hint line + "+Neu" row).
func (m Model) maxVisible() int {
	// chromeRows: top-border(1) + filter(1) + sep(1) + footer(1) + bot-border(1) = 5
	// Plus the "+Neu" row consumes one more slot.
	vis := m.height - chromeRows - 1
	if vis < 1 {
		vis = 1
	}
	return vis
}

// scrollOffset computes the scroll offset so the cursor stays within the
// visible window.
func (m Model) scrollOffset(maxVis int) int {
	offset := 0
	// Only the real items are scrollable ("+Neu" is always rendered outside
	// the scroll window).
	if m.cursor < len(m.filtered) {
		if m.cursor >= maxVis {
			offset = m.cursor - maxVis + 1
		}
	}
	return offset
}

// itoa converts an int to its decimal string without importing strconv
// directly in this file. Used for scroll-overflow labels ("3 weitere…").
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(append(b, digits...))
}
