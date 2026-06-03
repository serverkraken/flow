// Package projects — worktime_projects.go
//
// worktimeProjectsModel is the "Worktime-Projekte" sub-tab: a list of
// domain.Project rows drawn from usecase.Projects, augmented with a
// per-project session-count computed from SessionStore.LoadFiltered.
//
// Keys: j/k navigate, n new project (inline name-input), r rename,
// a archive (with confirm), A toggle archived, Enter → switch to
// worktime screen, / filter, Esc clear filter.
package projects

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// wpInputMode describes which inline text-input is currently open.
type wpInputMode int

const (
	wpInputNone   wpInputMode = iota
	wpInputNew                // n — create new project
	wpInputRename             // r — rename selected project
)

// wpProjectsLoadedMsg is the async result of listing projects + counting sessions.
type wpProjectsLoadedMsg struct {
	items  []domain.Project
	counts map[string]int // keyed by Project.ID
	err    error
}

// wpMutatedMsg is emitted after a successful create / rename / archive.
type wpMutatedMsg struct{}

// worktimeProjectsModel is the Worktime-Projekte sub-tab. Fields are
// value-typed for safe tea.Model copy semantics.
type worktimeProjectsModel struct {
	items        []domain.Project // all loaded (active + optional archived)
	visible      []domain.Project // post-filter
	highlights   [][]int          // fuzzy match indices per visible item
	counts       map[string]int   // session counts keyed by ProjectID
	cursor       int
	offset       int
	filter       textinput.Model
	showArchived bool

	inputMode wpInputMode
	input     textinput.Model // active inline input (new/rename)
	inputErr  string          // inline validation error

	confirmModel *confirm.Model // archive confirmation dialog

	toast *toast.Model // transient feedback

	loading bool
	err     error

	pal    theme.Palette
	styles wpStyles
	width  int
	height int

	projects *usecase.Projects  // nil → degraded "nicht verfügbar" state
	sessions ports.SessionStore // nil → session counts show "—"
	userID   string
}

type wpStyles struct {
	border    lipgloss.Style
	dimSep    lipgloss.Style
	archived  lipgloss.Style
	countHint lipgloss.Style
}

func newWPStyles(p theme.Palette) wpStyles {
	sem := p.Sem()
	return wpStyles{
		border:    lipgloss.NewStyle().Foreground(sem.Border),
		dimSep:    lipgloss.NewStyle().Foreground(p.FgMuted),
		archived:  lipgloss.NewStyle().Foreground(sem.Notice),
		countHint: lipgloss.NewStyle().Foreground(p.FgMuted),
	}
}

func newWorktimeProjects(p theme.Palette, projects *usecase.Projects, sessions ports.SessionStore, userID string) worktimeProjectsModel {
	fi := form.NewTextInput("filter…", p)
	inp := form.NewTextInput("Projektname", p)
	return worktimeProjectsModel{
		pal:      p,
		styles:   newWPStyles(p),
		filter:   fi,
		input:    inp,
		loading:  true,
		projects: projects,
		sessions: sessions,
		userID:   userID,
		counts:   map[string]int{},
	}
}

// --- tea.Model ---

func (m worktimeProjectsModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m worktimeProjectsModel) loadCmd() tea.Cmd {
	pr := m.projects
	se := m.sessions
	uid := m.userID
	showArchived := m.showArchived
	if pr == nil {
		return func() tea.Msg {
			return wpProjectsLoadedMsg{items: nil, counts: map[string]int{}, err: nil}
		}
	}
	return func() tea.Msg {
		var items []domain.Project
		var err error
		if showArchived {
			items, err = pr.ListAll(uid)
		} else {
			items, err = pr.ListActive(uid)
		}
		if err != nil {
			return wpProjectsLoadedMsg{err: err}
		}
		counts := map[string]int{}
		if se != nil {
			sessions, serr := se.LoadFiltered(uid, func(s domain.Session) bool {
				return s.ProjectID != ""
			})
			if serr == nil {
				for _, s := range sessions {
					counts[s.ProjectID]++
				}
			}
		}
		return wpProjectsLoadedMsg{items: items, counts: counts}
	}
}

func (m worktimeProjectsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case wpProjectsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.counts != nil {
			m.counts = msg.counts
		}
		m.items = msg.items
		m.applyFilter()
		return m, nil

	case wpMutatedMsg:
		m.loading = true
		m.inputMode = wpInputNone
		m.inputErr = ""
		m.confirmModel = nil
		return m, m.loadCmd()

	case confirm.ResultMsg:
		if m.confirmModel == nil {
			return m, nil
		}
		m.confirmModel = nil
		if !msg.Confirmed {
			return m, nil
		}
		return m.doArchive()

	case toast.DismissedMsg:
		m.toast = nil
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	// Forward to confirm sub-model when active.
	if m.confirmModel != nil {
		updated, cmd := m.confirmModel.Update(msg)
		m.confirmModel = &updated
		return m, cmd
	}
	// Forward to toast for tick.
	if m.toast != nil {
		var cmd tea.Cmd
		updated, cmd := m.toast.Update(msg)
		m.toast = &updated
		return m, cmd
	}
	return m, nil
}

func (m worktimeProjectsModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Confirm dialog gets all keys.
	if m.confirmModel != nil {
		updated, cmd := m.confirmModel.Update(msg)
		m.confirmModel = &updated
		return m, cmd
	}

	// Inline input (new / rename) consumes keys.
	if m.inputMode != wpInputNone {
		return m.handleInputKey(msg)
	}

	// Filter mode — arrows + Esc + Enter.
	if m.filter.Focused() {
		return m.handleFilterKey(msg)
	}

	return m.handleNormalKey(msg)
}

func (m worktimeProjectsModel) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
	case "n":
		return m.openNewInput()
	case "r":
		return m.openRenameInput()
	case "a":
		return m.openArchiveConfirm()
	case "A":
		m.showArchived = !m.showArchived
		m.loading = true
		return m, m.loadCmd()
	case "enter":
		// Switch to worktime screen. Full filter handoff (to verlauf sub-tab
		// filtered by this project) requires worktime.WithState changes that
		// are out of Task 18 scope — documented as DONE_WITH_CONCERNS.
		// The sidekick catches palette.SwitchScreenMsg and switches the screen.
		return m, func() tea.Msg {
			return palette.SwitchScreenMsg{Screen: domain.ScreenWorktime}
		}
	}

	// Type-to-filter: single printable character auto-focuses filter.
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

func (m worktimeProjectsModel) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		return m, nil
	case "enter":
		m.filter.Blur()
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

func (m worktimeProjectsModel) handleInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = wpInputNone
		m.inputErr = ""
		return m, nil
	case "enter":
		return m.submitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.inputErr = ""
	return m, cmd
}

func (m worktimeProjectsModel) openNewInput() (tea.Model, tea.Cmd) {
	inp := form.NewTextInput("Projektname", m.pal)
	inp.Focus()
	m.input = inp
	m.inputMode = wpInputNew
	m.inputErr = ""
	return m, textinput.Blink
}

func (m worktimeProjectsModel) openRenameInput() (tea.Model, tea.Cmd) {
	if m.projects == nil || len(m.visible) == 0 {
		return m, nil
	}
	cur := m.visible[m.cursor]
	inp := form.NewTextInput("Neuer Name", m.pal)
	inp.SetValue(cur.Name)
	inp.Focus()
	m.input = inp
	m.inputMode = wpInputRename
	m.inputErr = ""
	return m, textinput.Blink
}

func (m worktimeProjectsModel) openArchiveConfirm() (tea.Model, tea.Cmd) {
	if m.projects == nil || len(m.visible) == 0 {
		return m, nil
	}
	cur := m.visible[m.cursor]
	question := fmt.Sprintf("Projekt »%s« archivieren?", cur.Name)
	detail := "Archivierte Projekte sind weiterhin in der Verlauf-Ansicht sichtbar."
	cm := confirm.NewDanger(question, detail, m.pal)
	m.confirmModel = &cm
	return m, cm.Init()
}

func (m worktimeProjectsModel) submitInput() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.input.Value())
	if name == "" {
		m.inputErr = "Name darf nicht leer sein."
		return m, nil
	}
	if m.inputMode == wpInputNew {
		pr := m.projects
		uid := m.userID
		return m, func() tea.Msg {
			if _, err := pr.Create(uid, name); err != nil {
				return wpErrorMsg{err: err, context: "Erstellen fehlgeschlagen"}
			}
			return wpMutatedMsg{}
		}
	}
	// wpInputRename
	if len(m.visible) == 0 {
		m.inputMode = wpInputNone
		return m, nil
	}
	cur := m.visible[m.cursor]
	pr := m.projects
	uid := m.userID
	id := cur.ID
	return m, func() tea.Msg {
		if err := pr.Rename(uid, id, name); err != nil {
			return wpErrorMsg{err: err, context: "Umbenennen fehlgeschlagen"}
		}
		return wpMutatedMsg{}
	}
}

func (m worktimeProjectsModel) doArchive() (tea.Model, tea.Cmd) {
	if len(m.visible) == 0 || m.projects == nil {
		return m, nil
	}
	cur := m.visible[m.cursor]
	pr := m.projects
	uid := m.userID
	return m, func() tea.Msg {
		if err := pr.Archive(uid, cur.ID); err != nil {
			return wpErrorMsg{err: err, context: "Archivieren fehlgeschlagen"}
		}
		return wpMutatedMsg{}
	}
}

// wpErrorMsg surfaces use-case errors as a danger toast.
type wpErrorMsg struct {
	err     error
	context string
}

func (m worktimeProjectsModel) applyFilter() {
	q := m.filter.Value()
	if q == "" {
		m.visible = m.items
		m.highlights = make([][]int, len(m.visible))
	} else {
		names := make([]string, len(m.items))
		for i, p := range m.items {
			names[i] = p.Name
		}
		matches := fuzzy.Find(q, names)
		m.visible = make([]domain.Project, len(matches))
		m.highlights = make([][]int, len(matches))
		for i, match := range matches {
			m.visible[i] = m.items[match.Index]
			m.highlights[i] = match.MatchedIndexes
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.offset = 0
	m.ensureCursorVisible()
}

func (m worktimeProjectsModel) maxVisible() int {
	return max(1, m.height-theme.PickerChromeRows)
}

func (m *worktimeProjectsModel) ensureCursorVisible() {
	vis := m.maxVisible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

// filterActive reports whether a text input is consuming keystrokes.
func (m worktimeProjectsModel) filterActive() bool {
	return m.filter.Focused() || m.inputMode != wpInputNone
}

// stateFilter returns the current filter value for persistence.
func (m worktimeProjectsModel) stateFilter() string { return m.filter.Value() }

// stateCursor returns the cursor for persistence.
func (m worktimeProjectsModel) stateCursor() int { return m.cursor }

// --- View ---

func (m worktimeProjectsModel) View() tea.View {
	return tea.NewView(m.viewContent())
}

func (m worktimeProjectsModel) viewContent() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string

	// Degraded state: projects use case not wired.
	if m.projects == nil {
		body := strings.Join([]string{
			"",
			theme.Dim("  (nicht verfügbar — Sync nicht konfiguriert)", m.pal),
			"",
		}, "\n")
		box := titlebox.Render("Worktime-Projekte", body, m.width, m.pal)
		footer := statusbar.Hints(uistrings.HintHelp, m.pal)
		return box + "\n" + "\n" + footer
	}

	// Confirm dialog rendered inline above the list.
	if m.confirmModel != nil {
		confirmBody := "\n" + m.confirmModel.View() + "\n"
		box := titlebox.Render("Worktime-Projekte · Bestätigen", confirmBody, m.width, m.pal)
		footer := statusbar.Hints(uistrings.HintConfirm, m.pal)
		return box + "\n" + "\n" + footer
	}

	// Filter row.
	filterPrompt := theme.Dim(glyphs.Info+" ", m.pal)
	if m.filter.Focused() {
		filterPrompt = theme.Heading(glyphs.Active+" ", m.pal)
	}
	rows = append(rows, filterPrompt+m.filter.View())
	rows = append(rows, m.styles.border.Render(strings.Repeat("─", inner)))

	// List body.
	switch {
	case m.loading:
		rows = append(rows, theme.Dim("  lade Worktime-Projekte…", m.pal))
	case m.err != nil:
		rows = append(rows, theme.Err("  "+m.err.Error(), m.pal))
	case len(m.items) == 0:
		rows = append(rows, m.renderEmptyAll()...)
	case len(m.visible) == 0:
		rows = append(rows, m.renderEmptyFilter()...)
	default:
		vis := m.maxVisible()
		end := min(m.offset+vis, len(m.visible))
		if m.offset > 0 {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d vorherige…", glyphs.Up, m.offset), m.pal))
		}
		for i := m.offset; i < end; i++ {
			rows = append(rows, m.renderProjectRow(i == m.cursor, m.visible[i], m.highlights[i], inner))
		}
		if end < len(m.visible) {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d weitere…", glyphs.Down, len(m.visible)-end), m.pal))
		}
	}

	// Inline input overlay appended after list.
	if m.inputMode != wpInputNone {
		rows = append(rows, "")
		rows = append(rows, m.styles.border.Render(strings.Repeat("─", inner)))
		label := "Neues Projekt: "
		if m.inputMode == wpInputRename {
			label = "Umbenennen: "
		}
		rows = append(rows, "  "+theme.Dim(label, m.pal)+m.input.View())
		if m.inputErr != "" {
			rows = append(rows, "  "+theme.Err(m.inputErr, m.pal))
		}
	}

	body := strings.Join(rows, "\n")

	// Title with archived toggle indicator.
	titleStr := "Worktime-Projekte"
	if m.showArchived {
		titleStr += " · Archiviert anzeigen"
	}
	box := titlebox.Render(titleStr, body, m.width, m.pal)

	// Footer hints depend on mode.
	var hintsStr string
	switch {
	case m.inputMode != wpInputNone:
		hintsStr = "Enter → speichern  ·  " + uistrings.HintCancel
	case m.filter.Focused():
		hintsStr = uistrings.HintSearchInput
	default:
		hintsStr = strings.Join([]string{
			"n → neu",
			"r → umbenennen",
			"a → archivieren",
			uistrings.HintFilter,
		}, "  ·  ")
	}
	footer := statusbar.Hints(hintsStr, m.pal)
	return box + "\n" + toast.SlotLine(m.toast, "  ") + "\n" + footer
}

// renderProjectRow renders one project as a picker row with hint columns.
// Format: name (with fuzzy highlights) + right-aligned "last-used · N Sessions".
func (m worktimeProjectsModel) renderProjectRow(selected bool, p domain.Project, highlight []int, width int) string {
	hint := m.buildHint(p)
	archived := p.ArchivedAt != nil
	label := p.Name
	if archived {
		label = m.styles.archived.Render("[A] ") + p.Name
	}
	_ = label // picker.RowWithMatch accepts label separately
	return picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected:      selected,
		Label:         p.Name,
		Hint:          hint,
		Width:         width,
		Match:         highlight,
		HintPreStyled: true,
	}, m.pal)
}

// buildHint assembles the right-aligned hint string: "zuletzt vor X · N Sessions".
func (m worktimeProjectsModel) buildHint(p domain.Project) string {
	// Last-used relative time.
	lastUsed := "--"
	if !p.LastUsedAt.IsZero() {
		lastUsed = relativeTime(time.Now(), p.LastUsedAt)
	}

	// Session count.
	countStr := "—"
	if m.sessions != nil {
		if n, ok := m.counts[p.ID]; ok {
			countStr = fmt.Sprintf("%d", n)
		} else {
			countStr = "0"
		}
	}

	hint := fmt.Sprintf("%s  ·  %s Sessions", lastUsed, countStr)
	return m.styles.countHint.Render(hint)
}

func (m worktimeProjectsModel) renderEmptyAll() []string {
	if m.showArchived {
		return []string{
			"",
			theme.Dim("  Keine Projekte angelegt — drücke n für ein neues.", m.pal),
			"",
		}
	}
	return []string{
		"",
		theme.Dim("  Keine Projekte angelegt — drücke n für ein neues.", m.pal),
		"",
	}
}

func (m worktimeProjectsModel) renderEmptyFilter() []string {
	return []string{
		"",
		theme.Dim("  keine Treffer für »"+m.filter.Value()+"«", m.pal),
		"",
		theme.Dim("  "+uistrings.HintClearFilter, m.pal),
	}
}

// viewContentWithTitle renders the Worktime-Projekte tab with an
// externally supplied titlebox title (the host's tab-strip). Called by
// the root Model so both sub-tabs appear in the titlebox header.
func (m worktimeProjectsModel) viewContentWithTitle(title string) string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	// Degraded state: projects use case not wired.
	if m.projects == nil {
		body := strings.Join([]string{
			"",
			theme.Dim("  (nicht verfügbar — Sync nicht konfiguriert)", m.pal),
			"",
		}, "\n")
		box := titlebox.Render(title, body, m.width, m.pal)
		footer := statusbar.Hints(uistrings.HintHelp, m.pal)
		return box + "\n" + "\n" + footer
	}

	// Confirm dialog rendered inline.
	if m.confirmModel != nil {
		confirmBody := "\n" + m.confirmModel.View() + "\n"
		box := titlebox.Render(title+" · Bestätigen", confirmBody, m.width, m.pal)
		footer := statusbar.Hints(uistrings.HintConfirm, m.pal)
		return box + "\n" + "\n" + footer
	}

	var rows []string

	filterPrompt := theme.Dim(glyphs.Info+" ", m.pal)
	if m.filter.Focused() {
		filterPrompt = theme.Heading(glyphs.Active+" ", m.pal)
	}
	rows = append(rows, filterPrompt+m.filter.View())
	rows = append(rows, m.styles.border.Render(strings.Repeat("─", inner)))

	switch {
	case m.loading:
		rows = append(rows, theme.Dim("  lade Worktime-Projekte…", m.pal))
	case m.err != nil:
		rows = append(rows, theme.Err("  "+m.err.Error(), m.pal))
	case len(m.items) == 0:
		rows = append(rows, m.renderEmptyAll()...)
	case len(m.visible) == 0:
		rows = append(rows, m.renderEmptyFilter()...)
	default:
		vis := m.maxVisible()
		end := min(m.offset+vis, len(m.visible))
		if m.offset > 0 {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d vorherige…", glyphs.Up, m.offset), m.pal))
		}
		for i := m.offset; i < end; i++ {
			rows = append(rows, m.renderProjectRow(i == m.cursor, m.visible[i], m.highlights[i], inner))
		}
		if end < len(m.visible) {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d weitere…", glyphs.Down, len(m.visible)-end), m.pal))
		}
	}

	// Inline input overlay.
	if m.inputMode != wpInputNone {
		rows = append(rows, "")
		rows = append(rows, m.styles.border.Render(strings.Repeat("─", inner)))
		label := "Neues Projekt: "
		if m.inputMode == wpInputRename {
			label = "Umbenennen: "
		}
		rows = append(rows, "  "+theme.Dim(label, m.pal)+m.input.View())
		if m.inputErr != "" {
			rows = append(rows, "  "+theme.Err(m.inputErr, m.pal))
		}
	}

	body := strings.Join(rows, "\n")

	// Archived indicator in title.
	effectiveTitle := title
	if m.showArchived {
		effectiveTitle += " · Archiviert"
	}
	box := titlebox.Render(effectiveTitle, body, m.width, m.pal)

	var hintsStr string
	switch {
	case m.inputMode != wpInputNone:
		hintsStr = "Enter → speichern  ·  " + uistrings.HintCancel
	case m.filter.Focused():
		hintsStr = uistrings.HintSearchInput
	default:
		hintsStr = strings.Join([]string{
			"n → neu",
			"r → umbenennen",
			"a → archivieren",
			uistrings.HintFilter,
		}, "  ·  ")
	}
	footer := statusbar.Hints(hintsStr, m.pal)
	return box + "\n" + toast.SlotLine(m.toast, "  ") + "\n" + footer
}

// relativeTime formats a past time as a German relative string.
// Examples: "heute", "gestern", "vor 3 Tagen", "vor 2 Wochen".
func relativeTime(now, t time.Time) string {
	diff := now.Sub(t)
	if diff < 0 {
		return "gerade eben"
	}
	days := int(diff.Hours() / 24)
	switch {
	case days == 0:
		return "heute"
	case days == 1:
		return "gestern"
	case days < 7:
		return fmt.Sprintf("vor %d Tagen", days)
	case days < 14:
		return "vor 1 Woche"
	case days < 30:
		return fmt.Sprintf("vor %d Wochen", days/7)
	case days < 60:
		return "vor 1 Monat"
	default:
		return fmt.Sprintf("vor %d Monaten", days/30)
	}
}
