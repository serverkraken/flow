// Package palette implements the palette screen: a fuzzy-filterable,
// section-grouped list of all actions aggregated from enabled tmux
// plugins' menu.entries files.
//
// The screen is port-driven: PaletteReader loads + ranks entries,
// PaletteWriter mutates the persisted stats (Mark, TogglePin), and
// ports.Tmux dispatches the selected action via tmux run-shell. The
// pure ranking algorithm lives in domain.SortPaletteEntries.
package palette

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type loadedMsg struct {
	snapshot *usecase.PaletteSnapshot
	err      error
}

// dispatchedMsg fires after an external action (popup, fire-and-forget tmux
// command) was handed off to tmux. The palette stays open and shows a toast
// with the entry's label so the user gets confirmation. Pre-F-WAVE-1 this
// returned tea.Quit; that killed flow's process and made the surrounding
// sidekick pane flicker on every action.
//
// err is non-nil when RunShell or the persist call failed. The view
// surfaces it as a danger toast so the user knows the action did NOT
// take effect — silent failures previously left the user thinking the
// pin / mark / dispatch had succeeded.
type dispatchedMsg struct {
	label string
	err   error
}

// persistDoneMsg fires after a fire-and-forget persist (Mark or
// TogglePin). On error it surfaces a warning toast and keeps the UI in
// sync with reality by reloading; on success the reload picks up the
// new pin/usage state. Both Mark and TogglePin used to run inside
// Update (synchronous disk I/O on the bubbletea event loop) — a hung
// disk would freeze the whole UI.
type persistDoneMsg struct {
	err error
}

// SwitchScreenMsg is emitted when a palette entry's action is recognized as
// a flow-internal screen switch (the goto.sh deep-link pattern). The
// sidekick root catches it and updates m.current — no subshell, no flow
// restart, no flicker. Action strings that do NOT match this pattern fall
// through to the external dispatch path.
type SwitchScreenMsg struct {
	Screen string
}

// gotoScreenRe matches the action string written by ~/.tmux/plugins/flow/goto.sh.
// Examples it must catch:
//
//	run-shell '~/.tmux/plugins/flow/goto.sh worktime'
//	run-shell "~/.tmux/plugins/flow/goto.sh projects"
//	run-shell ~/.tmux/plugins/flow/goto.sh palette
//
// The captured group is the screen name (palette / projects / worktime /
// cheatsheet / notes), validated against domain.IsValidScreen at use site.
var gotoScreenRe = regexp.MustCompile(`flow/goto\.sh\s+(\w+)`)

// Model is the bubbletea model for the palette screen.
type Model struct {
	all        []domain.PaletteEntry
	visible    []domain.PaletteEntry
	highlights [][]int // label-rune-indices to highlight per visible entry
	cursor     int
	offset     int
	filter     textinput.Model
	pal        theme.Palette
	width      int
	height     int
	err        error
	loading    bool
	session    string
	stats      domain.PaletteStats

	// toast renders a transient ack after a non-screen-switch dispatch.
	// nil when no toast is active.
	toast *toast.Model

	reader *usecase.PaletteReader
	writer *usecase.PaletteWriter
	tmux   ports.Tmux
}

// New constructs a palette Model wired against the given use cases and
// tmux dispatcher.
func New(p theme.Palette, reader *usecase.PaletteReader, writer *usecase.PaletteWriter, tmux ports.Tmux) Model {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80
	return Model{
		pal:     p,
		filter:  ti,
		loading: true,
		reader:  reader,
		writer:  writer,
		tmux:    tmux,
	}
}

// HelpSections returns the canonical key bindings of the palette
// screen for aggregation by the sidekick `?`-overlay. Single source
// of truth — the overlay used to maintain a parallel hand-pasted copy.
func (Model) HelpSections() []help.Section {
	return []help.Section{{
		Title: "Palette",
		Keys: [][2]string{
			{"a–z (außer j/k/g/G)", "tippen → Filter direkt"},
			{"/", "Filter explizit öffnen"},
			{"j / k / ↑ / ↓", "Navigieren"},
			{"G / g", "Ende / Anfang"},
			{"] / [", "Nächste / vorige Section"},
			{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
			{"1–9", "Direktwahl (n-ter Treffer)"},
			{".", "Pin / Unpin (→ Favoriten)"},
			{"Enter", "Ausführen"},
			{"Esc", "Filter leeren · 2× → schließen"},
		},
	}}
}

// FilterActive reports whether the text input has focus.
func (m Model) FilterActive() bool { return m.filter.Focused() }

// StateFilter returns the current filter value for state persistence.
func (m Model) StateFilter() string { return m.filter.Value() }

// StateCursor returns the cursor position for state persistence.
func (m Model) StateCursor() int { return m.cursor }

// WithState restores filter and cursor from persisted state. Returns
// tea.Model (not the concrete type) so the sidekick root can call this
// through its stateRestorer interface.
func (m Model) WithState(filter string, cursor int) tea.Model {
	m.filter.SetValue(filter)
	m.cursor = cursor
	return m
}

// Init kicks off the async palette load.
func (m Model) Init() tea.Cmd { return m.loadCmd() }

func (m Model) loadCmd() tea.Cmd {
	r := m.reader
	return func() tea.Msg {
		snap, err := r.Load()
		return loadedMsg{snapshot: snap, err: err}
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
		if msg.snapshot != nil {
			m.all = msg.snapshot.Entries
			m.stats = msg.snapshot.Stats
			m.session = msg.snapshot.SessionName
		}
		m.applyFilter()
		return m, nil

	case dispatchedMsg:
		// External action handed off to tmux — stay in palette, toast.
		// On error, switch the toast to danger styling so the user
		// knows the action didn't actually run.
		var t toast.Model
		if msg.err != nil {
			t = toast.NewDanger("dispatch fehlgeschlagen: "+msg.err.Error(), m.pal)
		} else {
			t = toast.New("ausgeführt: "+msg.label, 2*time.Second, m.pal)
		}
		m.toast = &t
		return m, t.Init()

	case persistDoneMsg:
		if msg.err != nil {
			t := toast.NewWarning("persist fehlgeschlagen: "+msg.err.Error(), m.pal)
			m.toast = &t
			return m, tea.Batch(m.loadCmd(), t.Init())
		}
		return m, m.loadCmd()

	case toast.DismissedMsg:
		m.toast = nil
		return m, nil

	case tea.KeyMsg:
		if m.filter.Focused() {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

// handleNormalKey routes a key when the filter does not have focus.
// The function is a flat dispatch table over a fixed command set; a
// split would hide the structure behind helper indirection.
//
//nolint:gocyclo
func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	switch s {
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
	case "esc":
		// No-op. Palette runs as a screen inside sidekick; tea.Quit
		// here would tear down the whole program. The sidekick host
		// owns the quit key (`q` / `ctrl+c` at sidekick/model.go:185)
		// and does not consume esc itself, so swallowing it here is
		// the right shape — the user explicitly pressed esc on a
		// no-modal screen, no action.
		return m, nil
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
			return m, m.togglePin(m.visible[m.cursor])
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

	// Type-to-filter: any other single printable character auto-focuses
	// the filter and routes the keystroke into it. Saves the explicit
	// "/" before searching. Special keys (tab, ctrl-combos, etc.) have
	// multi-char names and fall through unhandled.
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

// togglePin flips the pin bit asynchronously. The persist runs inside a
// tea.Cmd so a hung disk doesn't freeze the bubbletea event loop. On
// failure the cmd returns a persistDoneMsg whose Update branch surfaces
// a warning toast and reloads; on success it returns loadedMsg directly
// so the UI re-renders against authoritative state in one trip.
func (m Model) togglePin(target domain.PaletteEntry) tea.Cmd {
	w := m.writer
	r := m.reader
	return func() tea.Msg {
		if err := w.TogglePin(target); err != nil {
			return persistDoneMsg{err: err}
		}
		snap, err := r.Load()
		return loadedMsg{snapshot: snap, err: err}
	}
}

// jumpSection moves the cursor to the first entry of the next (dir=+1)
// or previous (dir=-1) section. When already mid-section, [ first jumps
// to the top of the current section, then to the previous one on a
// second press.
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
	start := m.cursor
	for start > 0 && sectionAt(start-1) == cur {
		start--
	}
	if start < m.cursor {
		m.cursor = start
		m.ensureCursorVisible()
		return
	}
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
		// Esc with a non-empty filter clears the value AND blurs so j/k
		// (which are normal-mode navigation keys) reach the list. The
		// older two-stage "clear, keep focus, second esc quits" combined
		// with the type-to-filter auto-focus made it impossible to
		// navigate without re-pressing `/` — and any printable key then
		// re-trapped the user. Esc on an empty filter still quits.
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.filter.Blur()
			m.applyFilter()
			return m, nil
		}
		// Empty filter + esc → blur and stay on the screen. tea.Quit
		// here would tear down the sidekick host (palette is never
		// run standalone). The host's q-key owns program quit.
		m.filter.Blur()
		return m, nil
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
		m.visible = make([]domain.PaletteEntry, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.visible[i] = m.all[match.Index]
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

// dispatch records the pick via the writer (asynchronously inside the
// returned cmd so a hung disk doesn't freeze Update), then either:
//
//	(a) emits a SwitchScreenMsg if the action matches the goto.sh deep-link
//	    pattern — the sidekick root handles it as an in-process tab switch,
//	    no subshell, no flow restart;
//
//	(b) hands the action off to tmux via run-shell -b and returns a
//	    dispatchedMsg so the palette can toast confirmation while staying
//	    open. RunShell errors (tmux server down, malformed action) now
//	    surface as a danger toast — previously they were silently dropped
//	    and the user saw "ausgeführt" even when nothing ran. The earlier
//	    `sleep 0.15` prefix is gone: it was an undocumented workaround
//	    that introduced latency without solving any documented race.
//
// Pre-F-WAVE-1 every dispatch ended in tea.Quit, killing flow and forcing
// the sidekick pane to flicker on each action.
func (m Model) dispatch(e domain.PaletteEntry) tea.Cmd {
	w := m.writer
	tm := m.tmux
	entry := e

	if matches := gotoScreenRe.FindStringSubmatch(e.Action); matches != nil {
		screen := matches[1]
		if domain.IsValidScreen(screen) {
			return func() tea.Msg {
				_ = w.Mark(entry) // persist err is non-fatal for screen-switch
				return SwitchScreenMsg{Screen: screen}
			}
		}
	}

	action := e.Action
	label := e.Label
	return func() tea.Msg {
		_ = w.Mark(entry) // persist err is folded into dispatchedMsg.err below if RunShell also fails
		err := tm.RunShell(fmt.Sprintf("tmux %s", action))
		return dispatchedMsg{label: label, err: err}
	}
}

// View renders the palette screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string

	prompt := theme.Heading("› ", m.pal)
	rows = append(rows, prompt+m.filter.View())
	rows = append(rows, lipgloss.NewStyle().Foreground(m.pal.BgCode).
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
	prefix := "  ▸ "
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
	arrowStyle := lipgloss.NewStyle().Foreground(m.pal.BgCode)
	textStyle := lipgloss.NewStyle().Foreground(m.pal.FgMuted)
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
	hints := []string{
		"enter → ausführen",
		"j/k → bewegen",
		"/ → filter",
		"? → hilfe",
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
	if selected {
		bar = lipgloss.NewStyle().Foreground(m.pal.Sem().Accent).Render(picker.AccentBarRune)
		labelStyle = labelStyle.Bold(true)
	}
	matchStyle := lipgloss.NewStyle().Foreground(m.pal.Sem().Accent).Bold(true)
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
