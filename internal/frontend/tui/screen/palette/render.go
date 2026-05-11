package palette

// Palette rendering — View und alle View-Helpers (Title, Preview,
// Empty-State, Footer, Entry-Listing, Row-Painting). Split aus
// model.go (Skill §No-Monoliths): Rendering trennt sich sauber von
// Update-Routing und Type-Definitionen.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// View renders the palette screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string

	// Focused: filled ▶ in Accent-Bold; unfocused: dim ›. Non-color
	// signal für Filter-Focus — reine Cursor-Blink-Sichtbarkeit reichte
	// bei statischen Screenshots / langsamem Cursor nicht.
	prompt := theme.Dim("› ", m.pal)
	if m.filter.Focused() {
		prompt = theme.Heading("▶ ", m.pal)
	}
	rows = append(rows, prompt+m.filter.View())
	rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.Sem().Border).
		Render(strings.Repeat("─", inner)))

	switch {
	case m.loading:
		rows = append(rows, theme.Dim("  Aktionen werden geladen…", m.pal))
	case m.err != nil:
		rows = append(rows, theme.Err("  "+m.err.Error(), m.pal))
	case len(m.visible) == 0:
		rows = append(rows, m.renderEmptyState()...)
	default:
		rows = append(rows, m.renderEntries(inner)...)
	}

	body := strings.Join(rows, "\n")
	box := titlebox.Render(m.title(), body, m.width, m.pal)
	parts := []string{box}
	if preview := m.renderPreview(m.width - 4); preview != "" {
		parts = append(parts, preview)
	}
	// Reserved toast slot: always one line, blank when no toast is
	// active. Keeps the footer at the same screen row regardless of
	// toast state — the user's eye doesn't need to re-acquire it.
	parts = append(parts, toast.SlotLine(m.toast, "  "))
	parts = append(parts, m.renderFooter())
	return strings.Join(parts, "\n")
}

func (m Model) renderPreview(maxWidth int) string {
	if len(m.visible) == 0 || m.loading || m.err != nil {
		return ""
	}
	action := m.visible[m.cursor].Action
	prefix := "  " + glyphs.Active + " "
	available := maxWidth - lipgloss.Width(prefix)
	if available < 8 {
		return ""
	}
	if lipgloss.Width(action) > available {
		runes := []rune(action)
		if available-1 < len(runes) {
			action = string(runes[:available-1]) + "…"
		}
	}
	arrowStyle := lipgloss.NewStyle().Foreground(m.pal.Sem().Border)
	textStyle := lipgloss.NewStyle().Foreground(m.pal.FgMuted)
	return "  " + arrowStyle.Render(glyphs.Active) + " " + textStyle.Render(action)
}

func (m Model) title() string {
	parts := []string{"Palette"}
	if m.session != "" {
		parts = append(parts, "session: "+m.session)
	}
	if m.filter.Value() != "" {
		parts = append(parts, fmt.Sprintf("%d/%d Aktionen", len(m.visible), len(m.all)))
	} else {
		parts = append(parts, fmt.Sprintf("%d Aktionen", len(m.all)))
	}
	return strings.Join(parts, " · ")
}

func (m Model) renderEmptyState() []string {
	dim := lipgloss.NewStyle().Foreground(m.pal.FgMuted)
	if m.filter.Value() != "" {
		return []string{
			"",
			dim.Render("  keine Treffer für »" + m.filter.Value() + "«"),
			"",
			dim.Render("  esc → filter leeren  ·  ctrl+u → ganz zurücksetzen"),
		}
	}
	return []string{
		"",
		dim.Render("  noch keine Aktionen geladen — tmux-Plugins aktiviert?"),
	}
}

// renderFooter draws the canonical hint strip — max 4 most-frequent hints,
// `key → action` form, all-dim, separator `  ·  `. Surplus bindings (1-9
// direktwahl, ] / [ section jumps, `.` pin, esc two-stage) live in the `?`
// overlay rendered by the sidekick root.
func (m Model) renderFooter() string {
	// Enter und j/k bleiben palette-spezifisch — "ausführen" und
	// "bewegen" sind präziser als das generische strings.HintNav
	// ("Enter → wählen / j/k → navigieren") für den Action-Picker-
	// Kontext. / und ? kommen aus den kanonischen strings.*-Konstanten,
	// damit ein Wording-Drift mit anderen Footern direkt sichtbar wird.
	hints := []string{
		"Enter → ausführen",
		"j/k → bewegen",
		uistrings.HintFilter,
		uistrings.HintHelp,
	}
	return statusbar.Hints(strings.Join(hints, "  ·  "), m.pal)
}

func (m Model) renderEntries(innerWidth int) []string {
	vis := m.maxVisible()
	end := min(m.offset+vis, len(m.visible))

	sectionCounts := make(map[string]int)
	for _, e := range m.visible {
		sectionCounts[m.stats.EffectiveSection(e)]++
	}

	var rows []string
	if m.offset > 0 {
		rows = append(rows, theme.Dim(fmt.Sprintf("  ↑ %d vorherige…", m.offset), m.pal))
	}
	lastSection := ""
	for i := m.offset; i < end; i++ {
		e := m.visible[i]
		section := m.stats.EffectiveSection(e)
		if section != lastSection {
			if lastSection != "" {
				rows = append(rows, "")
			}
			header := fmt.Sprintf("%s · %d", section, sectionCounts[section])
			rows = append(rows, picker.SectionHeader(header, innerWidth, m.pal))
			lastSection = section
		}
		rows = append(rows, m.renderRow(i == m.cursor, e.Label, m.highlights[i], e.Keybind, innerWidth))
	}
	if end < len(m.visible) {
		rows = append(rows, theme.Dim(fmt.Sprintf("  ↓ %d weitere…", len(m.visible)-end), m.pal))
	}
	return rows
}

// renderRow paints one entry row. Mirrors picker.Row's layout (bar ·
// label · gap · hint) but supports per-rune highlight indices for
// fuzzy-match emphasis — picker.Row applies a single foreground style
// across the whole label and would overwrite our inline accent codes.
func (m Model) renderRow(selected bool, label string, highlight []int, hint string, width int) string {
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(m.pal.Fg)
	matchStyle := lipgloss.NewStyle().Foreground(m.pal.Sem().Accent).Bold(true)
	if selected {
		bar = lipgloss.NewStyle().Foreground(m.pal.Sem().Accent).Render(picker.AccentBarRune)
		labelStyle = labelStyle.Bold(true).Underline(true)
		matchStyle = matchStyle.Underline(true)
	}
	hintStyle := lipgloss.NewStyle().Foreground(m.pal.FgMuted)

	hi := make(map[int]bool, len(highlight))
	for _, idx := range highlight {
		hi[idx] = true
	}

	var b strings.Builder
	for i, r := range []rune(label) {
		if hi[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(labelStyle.Render(string(r)))
		}
	}
	rendered := b.String()

	gap := width - 1 - lipgloss.Width(label) - lipgloss.Width(hint) - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + rendered + strings.Repeat(" ", gap) + hintStyle.Render(hint)
}
