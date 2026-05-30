// Package projects implements the project-switcher screen: a
// fuzzy-filterable list of git repos under SOURCECODE_ROOT, annotated
// with which ones already have a tmux session attached.
//
// The screen is port-driven: ProjectsReader enumerates + tmux-annotates
// repos, ProjectSwitcher creates / attaches the session. No filesystem
// or exec.Command calls happen in this package.
package projects

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/usecase"
)

type loadedMsg struct {
	projects []domain.Project
	err      error
}

type switchedMsg struct {
	err error
}

// Model is the bubbletea model for the project-switcher screen.
//
// styles is built once at New() — projects mirrors palette's row
// contract (accent-bar + fuzzy-match emphasis) so the two sibling
// pickers read identically; the cache keeps the per-row render path
// allocation-free, same rationale as palette/Model.styles.
type Model struct {
	all        []domain.Project
	visible    []domain.Project
	highlights [][]int // name-rune-indices to emphasise per visible project
	cursor     int
	offset     int
	filter     textinput.Model
	pal        theme.Palette
	styles     projectsStyles
	width      int
	height     int
	err        error
	loading    bool
	rootDir    string

	// switchToast surfacet tmux-Dispatch-Fehler ohne den Body zu
	// überschreiben; ohne ihn ginge der Listen-Kontext (Cursor-Position,
	// gerade getippter Filter) verloren, sobald ein Switch fehlschlägt.
	switchToast *toast.Model

	reader   *usecase.ProjectsReader
	switcher *usecase.ProjectSwitcher

	mode Mode
}

// Mode discriminates the projects-screen's hosting context. In beiden
// Modes ist das funktionale Verhalten heute identisch (tea.Quit nach
// erfolgreichem Switch — tmux switch-client hat den Client schon
// umgehängt; der Sidekick wird im neuen Projekt-Kontext frisch
// initialisiert). Die Option existiert für API-Symmetrie mit palette
// (CLAUDE-tmux-migration-plan §3) und als Hook für künftige Standalone-
// only-Anpassungen (z. B. Banner-Text "popup schließt").
type Mode int

const (
	// ModeEmbedded ist das Standardverhalten — Projects läuft im Sidekick.
	ModeEmbedded Mode = iota
	// ModeStandalone ist für tmux-Popup-Aufruf via `flow projects`.
	ModeStandalone
)

// projectsStyles caches the palette-derived lipgloss styles used by the
// row renderer. Mirrors palette.paletteStyles so both pickers share the
// same visual contract; built once at New(), reused every frame.
type projectsStyles struct {
	label    lipgloss.Style
	labelSel lipgloss.Style
	match    lipgloss.Style
	matchSel lipgloss.Style
	bar      lipgloss.Style // AccentBarRune for the selected row
	border   lipgloss.Style // Sem().Border — filter separator rule
	marker   lipgloss.Style // Sem().Active — tmux-session hint glyph
}

func newProjectsStyles(p theme.Palette) projectsStyles {
	sem := p.Sem()
	label := lipgloss.NewStyle().Foreground(p.Fg)
	match := lipgloss.NewStyle().Foreground(sem.Accent).Bold(true)
	return projectsStyles{
		label:    label,
		labelSel: label.Bold(true).Underline(true),
		match:    match,
		matchSel: match.Underline(true),
		bar:      lipgloss.NewStyle().Foreground(sem.Accent),
		border:   lipgloss.NewStyle().Foreground(sem.Border),
		marker:   lipgloss.NewStyle().Foreground(sem.Active),
	}
}

// Option mutates a Model after New().
type Option func(*Model)

// WithStandalone schaltet den ModeStandalone — siehe Mode-Doku.
func WithStandalone() Option {
	return func(m *Model) { m.mode = ModeStandalone }
}

// New constructs a projects Model. rootDir is purely informational —
// shown in the title bar; the reader is responsible for honouring it.
func New(p theme.Palette, rootDir string, reader *usecase.ProjectsReader, switcher *usecase.ProjectSwitcher, opts ...Option) Model {
	ti := form.NewTextInput("filter…", p)
	m := Model{
		pal:      p,
		styles:   newProjectsStyles(p),
		filter:   ti,
		rootDir:  rootDir,
		loading:  true,
		reader:   reader,
		switcher: switcher,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// HelpSections exposes the projects-screen key bindings to the
// sidekick `?`-overlay aggregation. Source of truth — see palette/Model
// for the same pattern.
func (Model) HelpSections() []help.Section {
	return []help.Section{{
		Title: "Projekte",
		Keys: [][2]string{
			{"a–z (außer j/k/g/G)", "tippen → Filter direkt"},
			{"/", "Filter explizit öffnen"},
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"Esc", "Filter löschen"},
			{"Enter", "Wechseln"},
		},
	}}
}

// FilterActive reports whether the filter input is focused.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state. Returns
// tea.Model so the sidekick root can call through its stateRestorer
// interface.
func (m Model) WithState(filter string, cursor int) tea.Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init kicks off the async project enumeration.
func (m Model) Init() tea.Cmd {
	r := m.reader
	return func() tea.Msg {
		ps, err := r.List()
		return loadedMsg{projects: ps, err: err}
	}
}

// Update handles messages for the projects screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		m.all = msg.projects
		m.applyFilter()
		return m, nil

	case switchedMsg:
		// Surface tmux failures (no server, missing socket) instead of
		// quitting silently. Routed über einen Danger-Toast statt einer
		// Body-Fehlerzeile, damit die Projekt-Liste mit Cursor-Position
		// + Filter sichtbar bleibt — der User kann direkt erneut wählen.
		if msg.err != nil {
			t := toast.NewDanger("dispatch fehlgeschlagen: "+msg.err.Error(), m.pal)
			m.switchToast = &t
			return m, t.Init()
		}
		return m, tea.Quit

	case toast.DismissedMsg:
		m.switchToast = nil
		return m, nil

	case tea.KeyPressMsg:
		if m.filter.Focused() {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Every handled case returns explicitly so navigation keys (j/k/g/G/…)
	// never fall through into the type-to-filter block below — they are
	// single printable runes too (mirror palette/handleNormalKey).
	switch msg.String() {
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
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
			return m, m.switchToProject(m.visible[m.cursor])
		}
		return m, nil
	}

	// Type-to-filter: any other single printable character auto-focuses
	// the filter and routes the keystroke into it, saving the explicit
	// "/" before searching (mirror palette/handleNormalKey). Special keys
	// (tab, ctrl-combos, …) have multi-char names and fall through.
	s := msg.String()
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

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		return m, nil
	case "enter":
		m.filter.Blur()
		if len(m.visible) > 0 {
			return m, m.switchToProject(m.visible[m.cursor])
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
		names := make([]string, len(m.all))
		for i, p := range m.all {
			names[i] = p.Name
		}
		matches := fuzzy.Find(q, names)
		m.visible = make([]domain.Project, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
			m.highlights[i] = match.MatchedIndexes
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.offset = 0
	m.ensureCursorVisible()
}

func (m Model) maxVisible() int {
	return max(1, m.height-theme.PickerChromeRows)
}

func (m *Model) ensureCursorVisible() {
	vis := m.maxVisible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

// switchToProject delegates to the use case. On success the program
// quits so the operator sees the new tmux session in the foreground;
// on failure the err propagates back as switchedMsg.err and surfaces
// in the loaded-error row.
func (m Model) switchToProject(p domain.Project) tea.Cmd {
	sw := m.switcher
	return func() tea.Msg {
		return switchedMsg{err: sw.Switch(p)}
	}
}

// View renders the projects screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string
	// Focused: filled ▶ Accent-Bold; unfocused: dim › — non-color signal
	// für Filter-Focus, mirror palette/View.
	prompt := theme.Dim(glyphs.Info+" ", m.pal)
	if m.filter.Focused() {
		prompt = theme.Heading(glyphs.Active+" ", m.pal)
	}
	rows = append(rows, prompt+m.filter.View())
	// Separator rule under the filter — same chrome as palette/View so the
	// two pickers frame their input identically.
	rows = append(rows, m.styles.border.Render(strings.Repeat("─", inner)))

	switch {
	case m.loading:
		rows = append(rows, theme.Dim("  lade Projekte…", m.pal))
	case m.err != nil:
		rows = append(rows, theme.Err("  "+m.err.Error(), m.pal))
	case len(m.all) == 0:
		rows = append(rows, theme.Dim("  keine Projekte gefunden — $SOURCECODE_ROOT prüfen", m.pal))
	case len(m.visible) == 0:
		rows = append(rows, m.renderEmptyState()...)
	default:
		vis := m.maxVisible()
		end := min(m.offset+vis, len(m.visible))
		if m.offset > 0 {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d vorherige…", glyphs.Up, m.offset), m.pal))
		}
		for i := m.offset; i < end; i++ {
			rows = append(rows, m.renderRow(i == m.cursor, m.visible[i], m.highlights[i], inner))
		}
		if end < len(m.visible) {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d weitere…", glyphs.Down, len(m.visible)-end), m.pal))
		}
	}

	body := strings.Join(rows, "\n")
	label := lastSegment(m.rootDir)
	var title string
	if m.filter.Value() != "" {
		title = fmt.Sprintf("Projekte · %s · %d/%d", label, len(m.visible), len(m.all))
	} else {
		title = fmt.Sprintf("Projekte · %s · %d", label, len(m.all))
	}
	box := titlebox.Render(title, body, m.width, m.pal)
	// 4-Cap (skill §Spacing): wichtigste 4 im Footer, Surplus (esc/b/q) sind
	// Fixed-Slot-Keys, dokumentiert im sidekick-globalen `?`-Overlay.
	// Enter und j/k sind projects-spezifisch ("wechseln" / "bewegen") —
	// die generischen strings.HintNav-Wordings würden die Action verwischen.
	// / und ? kommen aus dem kanonischen strings-Vokabular.
	hints := strings.Join([]string{
		"Enter → wechseln",
		"j/k → bewegen",
		uistrings.HintFilter,
		uistrings.HintHelp,
	}, "  ·  ")
	footer := statusbar.Hints(hints, m.pal)
	// Toast-Slot zwischen Box und Footer: hält den Footer auf konstanter
	// Bildschirmzeile, egal ob ein Toast aktiv ist (mirror palette/View).
	return box + "\n" + toast.SlotLine(m.switchToast, "  ") + "\n" + footer
}

// renderRow paints one project row: accent bar, fuzzy-matched name with
// per-rune emphasis, and a right-aligned tmux-session marker. Mirrors
// palette/renderRow — the match emphasis is why projects can't use the
// plain picker.Row (which styles the whole label uniformly).
func (m Model) renderRow(selected bool, p domain.Project, highlight []int, width int) string {
	bar := " "
	labelStyle := m.styles.label
	matchStyle := m.styles.match
	if selected {
		bar = m.styles.bar.Render(picker.AccentBarRune)
		labelStyle = m.styles.labelSel
		matchStyle = m.styles.matchSel
	}

	hi := make(map[int]bool, len(highlight))
	for _, idx := range highlight {
		hi[idx] = true
	}
	var b strings.Builder
	for i, r := range []rune(p.Name) {
		if hi[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(labelStyle.Render(string(r)))
		}
	}
	rendered := b.String()

	// Active-tmux-session marker stays right-aligned in the hint slot so a
	// long name never pushes it off — the gap absorbs the slack.
	hint := ""
	if p.HasTmuxSession {
		hint = m.styles.marker.Render(glyphs.Active)
	}
	gap := width - 1 - lipgloss.Width(p.Name) - lipgloss.Width(hint) - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + rendered + strings.Repeat(" ", gap) + hint
}

// renderEmptyState is the no-match block: echoes the query plus the two
// recovery keys, mirroring palette/renderEmptyState so a fruitless filter
// always shows a way out instead of a bare "keine Einträge".
func (m Model) renderEmptyState() []string {
	return []string{
		"",
		theme.Dim("  keine Treffer für »"+m.filter.Value()+"«", m.pal),
		"",
		theme.Dim("  esc → filter leeren  ·  ctrl+u → ganz zurücksetzen", m.pal),
	}
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
