package palette

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/tui-kit/components/picker"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

// Internal messages.
type loadedMsg struct {
	entries []Entry
	stats   *Stats
	err     error
}

type dispatchedMsg struct{}

// Model is the bubbletea model for the palette screen.
type Model struct {
	all        []Entry
	visible    []Entry
	highlights [][]int // label-rune-indices to highlight per visible entry; aligned to visible
	cursor     int
	offset     int
	filter     textinput.Model
	theme      tk.Palette
	width      int
	height     int
	err        error
	loading    bool
	session    string
	stats      *Stats
}

// New creates a new palette Model.
func New(p tk.Palette) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80
	return Model{
		theme:   p,
		filter:  ti,
		loading: true,
		session: currentSession(),
	}
}

// currentSession reads the active tmux session name. Empty when tmux is unavailable.
func currentSession() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FilterActive reports whether the text input has focus.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter value for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor position for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state.
func (m Model) WithState(filter string, cursor int) Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init loads palette entries asynchronously.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		entries, stats, err := LoadEntries()
		return loadedMsg{entries: entries, stats: stats, err: err}
	}
}

// Update handles messages for the palette screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		m.all = msg.entries
		m.stats = msg.stats
		m.applyFilter()
		return m, nil

	case dispatchedMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		if m.filter.Focused() {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	switch s {
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
	case "esc":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
		return m, nil
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
		return m, nil
	case "G":
		m.cursor = max(0, len(m.visible)-1)
		m.ensureCursorVisible()
		return m, nil
	case "g":
		m.cursor = 0
		m.ensureCursorVisible()
		return m, nil
	case "pgdown", "ctrl+d":
		m.cursor = min(len(m.visible)-1, m.cursor+m.maxVisible())
		m.ensureCursorVisible()
		return m, nil
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-m.maxVisible())
		m.ensureCursorVisible()
		return m, nil
	case "enter":
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	case "]":
		m.jumpSection(+1)
		return m, nil
	case "[":
		m.jumpSection(-1)
		return m, nil
	case ".":
		if len(m.visible) > 0 {
			m.togglePin(m.visible[m.cursor])
		}
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(s[0] - '0')
		if n-1 < len(m.visible) {
			m.cursor = n - 1
			m.ensureCursorVisible()
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	}

	// Type-to-filter: any other single printable character auto-focuses the
	// filter and routes the keystroke into it. Cmd-K-style — saves the
	// explicit "/" before searching. Special keys (tab, ctrl-combos, etc.)
	// have multi-char names and fall through unhandled.
	if len(s) == 1 && s[0] >= ' ' && s[0] < 127 {
		m.filter.Focus()
		var cmd tea.Cmd
		prev := m.filter.Value()
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != prev {
			m.applyFilter()
		}
		return m, tea.Batch(cmd, textinput.Blink)
	}
	return m, nil
}

// togglePin flips the pin bit for an entry, persists, re-sorts, and follows
// the cursor onto the same entry in its new position. Pin-toggle has to
// reshuffle because pinned entries float into a virtual "Favoriten" section.
func (m *Model) togglePin(target Entry) {
	m.stats.TogglePin(target)
	_ = m.stats.Save()
	SortEntries(m.all, m.stats)
	m.applyFilter()
	for i, e := range m.visible {
		if entryKey(e) == entryKey(target) {
			m.cursor = i
			m.ensureCursorVisible()
			return
		}
	}
}

// jumpSection moves the cursor to the first entry of the next (dir=+1) or
// previous (dir=-1) section. When already mid-section, [ first jumps to the
// top of the current section, then to the previous one on a second press.
func (m *Model) jumpSection(dir int) {
	if len(m.visible) == 0 {
		return
	}
	sectionAt := func(i int) string { return m.stats.EffectiveSection(m.visible[i]) }
	cur := sectionAt(m.cursor)
	if dir > 0 {
		for i := m.cursor + 1; i < len(m.visible); i++ {
			if sectionAt(i) != cur {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
		return
	}
	// dir < 0 — first walk to start of current section
	start := m.cursor
	for start > 0 && sectionAt(start-1) == cur {
		start--
	}
	if start < m.cursor {
		m.cursor = start
		m.ensureCursorVisible()
		return
	}
	// already at top → jump to start of previous section
	if start == 0 {
		return
	}
	prev := sectionAt(start - 1)
	target := start - 1
	for target > 0 && sectionAt(target-1) == prev {
		target--
	}
	m.cursor = target
	m.ensureCursorVisible()
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Two-stage: first esc clears the value (keeping focus, so the user
		// can immediately retype); second esc on an empty filter closes the
		// palette. Keeps the editing flow snappy without making esc-to-quit
		// hard to reach.
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit
	case tea.KeyEnter:
		m.filter.Blur()
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

func (m *Model) applyFilter() {
	q := m.filter.Value()
	if q == "" {
		m.visible = m.all
		m.highlights = make([][]int, len(m.visible))
	} else {
		targets := make([]string, len(m.all))
		for i, e := range m.all {
			targets[i] = e.Section + " " + e.Label
		}
		matches := fuzzy.Find(q, targets)
		m.visible = make([]Entry, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
			// Re-find against label alone to get label-relative indices for
			// rendering. The primary match above can hit on section text;
			// a label-only re-match returns nil highlights then, which is
			// the desired behaviour (no false-positive characters lit up).
			labelHits := fuzzy.Find(q, []string{m.visible[i].Label})
			if len(labelHits) > 0 {
				m.highlights[i] = labelHits[0].MatchedIndexes
			}
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.offset = 0
	m.ensureCursorVisible()
}

func (m Model) maxVisible() int {
	return max(1, m.height-6)
}

func (m *Model) ensureCursorVisible() {
	vis := m.maxVisible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

func (m Model) dispatch(e Entry) tea.Cmd {
	m.stats.Mark(e)
	_ = m.stats.Save()
	return func() tea.Msg {
		cmd := exec.Command("tmux", "run-shell", "-b",
			fmt.Sprintf("sleep 0.15; tmux %s", e.Action))
		_ = cmd.Start()
		return dispatchedMsg{}
	}
}

// View renders the palette screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4 // box border (2) + padding (2)

	var rows []string

	// Filter bar.
	prompt := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("› ")
	filterLine := prompt + m.filter.View()
	rows = append(rows, filterLine)
	rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Border).
		Render(strings.Repeat("─", inner)))

	switch {
	case m.loading:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  lade…"))
	case m.err != nil:
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("  "+m.err.Error()))
	case len(m.visible) == 0:
		rows = append(rows, m.renderEmptyState(inner)...)
	default:
		rows = append(rows, m.renderEntries(inner)...)
	}

	body := strings.Join(rows, "\n")
	box := titlebox.Render(m.title(), body, m.width, m.theme)
	parts := []string{box}
	if preview := m.renderPreview(m.width - 4); preview != "" {
		parts = append(parts, preview)
	}
	parts = append(parts, m.renderFooter())
	return strings.Join(parts, "\n")
}

// renderPreview shows the tmux command bound to the highlighted entry. Helps
// with discoverability — what does this action actually do? — and doubles
// as a debug aid when an entry behaves unexpectedly. Empty when nothing is
// selected (loading / no matches), suppressed there to avoid empty space.
func (m Model) renderPreview(maxWidth int) string {
	if len(m.visible) == 0 || m.loading || m.err != nil {
		return ""
	}
	action := m.visible[m.cursor].Action
	prefix := "  ▸ "
	available := maxWidth - lipgloss.Width(prefix)
	if available < 8 {
		return ""
	}
	if lipgloss.Width(action) > available {
		runes := []rune(action)
		// Trim by runes; lipgloss.Width is rune-aware so this stays roughly
		// in sync for ASCII and CJK alike. Off-by-one is fine — the ellipsis
		// is the visual cue.
		if available-1 < len(runes) {
			action = string(runes[:available-1]) + "…"
		}
	}
	arrowStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
	textStyle := lipgloss.NewStyle().Foreground(m.theme.Dim)
	return "  " + arrowStyle.Render("▸") + " " + textStyle.Render(action)
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

func (m Model) renderEmptyState(_ int) []string {
	dim := lipgloss.NewStyle().Foreground(m.theme.Dim)
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

func (m Model) renderFooter() string {
	hints := [][2]string{
		{"enter", "ausführen"},
		{"1-9", "direktwahl"},
		{"j/k", "bewegen"},
		{"]/[", "section"},
		{".", "pin"},
		{"esc", "zurück"},
		{"q", "schließen"},
	}
	keyStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(m.theme.Dim)
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, keyStyle.Render(h[0])+" "+descStyle.Render(h[1]))
	}
	return lipgloss.NewStyle().Padding(0, 1).
		Render(strings.Join(parts, sepStyle.Render(" · ")))
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
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).
			Render(fmt.Sprintf("  ↑ %d vorherige…", m.offset)))
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
			rows = append(rows, picker.SectionHeader(header, innerWidth, m.theme))
			lastSection = section
		}
		rows = append(rows, m.renderRow(i == m.cursor, e.Label, m.highlights[i], e.Keybind, innerWidth))
	}
	if end < len(m.visible) {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Dim).
			Render(fmt.Sprintf("  ↓ %d weitere…", len(m.visible)-end)))
	}
	return rows
}

const accentBarRune = "▎"

// renderRow paints one entry row. Mirrors picker.Row's layout (bar · label
// · gap · hint) but supports per-rune highlight indices for fuzzy-match
// emphasis — picker.Row applies a single foreground style across the whole
// label and would overwrite our inline accent codes.
func (m Model) renderRow(selected bool, label string, highlight []int, hint string, width int) string {
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.Fg)
	if selected {
		bar = lipgloss.NewStyle().Foreground(m.theme.Accent).Render(accentBarRune)
		labelStyle = labelStyle.Bold(true)
	}
	matchStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(m.theme.Dim)

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
