// Package projects implements the project-switcher screen: two sub-tabs,
// "Quellverzeichnisse" (fuzzy-filterable SourceDir listing) and
// "Worktime-Projekte" (manage domain.Project rows).
//
// The root Model is the sub-tab host; it satisfies the sidekick's
// subTabHost interface so the sidekick can route numeric keys 1-2 to
// SwitchSubTab. Each sub-tab draws its own titlebox with the tab-strip
// as the title (same pattern as worktime/model.go).
//
// Deps wiring: Projects + Sessions + UserID are nullable — when nil /
// empty the Worktime-Projekte tab renders a "nicht verfügbar" note.
// cmd/flow/main.go wires these in Task 32.
package projects

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// tab identifies which of the two sub-tabs is active.
type tab int

const (
	tabSourceDirs tab = 0
	tabWTProjects tab = 1
)

// Model is the root bubbletea model for the projects screen.
//
// It holds two sub-models as opaque tea.Model values:
//   - subs[0] — sourceDirsModel (Quellverzeichnisse)
//   - subs[1] — worktimeProjectsModel (Worktime-Projekte)
//
// The root satisfies sidekick.subTabHost (SubTabs / SubTabIndex /
// SwitchSubTab) so the sidekick routes numeric keys 1-2 to the host.
type Model struct {
	pal     theme.Palette
	width   int
	height  int
	current tab
	subs    [2]tea.Model
}

// Mode discriminates the projects-screen's hosting context. Kept for
// API-symmetry with the old single-tab Model and for WithStandalone.
type Mode int

const (
	// ModeEmbedded ist das Standardverhalten — Projects läuft im Sidekick.
	ModeEmbedded Mode = iota
	// ModeStandalone ist für tmux-Popup-Aufruf via `flow projects`.
	ModeStandalone
)

// Option mutates a Model after New().
type Option func(*Model)

// WithStandalone schaltet den ModeStandalone — see Mode docs.
func WithStandalone() Option {
	return func(m *Model) {
		// Propagate to the source dirs sub-model.
		if sd, ok := m.subs[tabSourceDirs].(sourceDirsModel); ok {
			sd.mode = ModeStandalone
			m.subs[tabSourceDirs] = sd
		}
	}
}

// New constructs the projects host Model with both sub-tabs.
//
//   - rootDir: informational, shown in the Quellverzeichnisse title.
//   - reader / switcher: drive the Quellverzeichnisse sub-tab.
//   - projects / sessions / userID: drive the Worktime-Projekte sub-tab.
//     All three are nullable — the tab degrades gracefully.
func New(
	p theme.Palette,
	rootDir string,
	reader *usecase.ProjectsReader,
	switcher *usecase.ProjectSwitcher,
	opts ...Option,
) Model {
	return NewWithDeps(p, rootDir, reader, switcher, nil, nil, "", opts...)
}

// NewWithDeps is the full constructor. cmd/flow/main.go calls this once
// projects + sessions deps are wired (Task 32). New calls through with nil
// for projects/sessions and "" for userID so legacy callers compile unchanged.
func NewWithDeps(
	p theme.Palette,
	rootDir string,
	reader *usecase.ProjectsReader,
	switcher *usecase.ProjectSwitcher,
	projects *usecase.Projects,
	sessions ports.SessionStore,
	userID string,
	opts ...Option,
) Model {
	m := Model{
		pal: p,
		subs: [2]tea.Model{
			newSourceDirs(p, rootDir, reader, switcher, ModeEmbedded),
			newWorktimeProjects(p, projects, sessions, userID),
		},
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// --- sidekick.subTabHost interface ---

// SubTabs returns the two sub-tab labels in display order.
func (m Model) SubTabs() []string {
	return []string{"Quellverzeichnisse", "Worktime-Projekte"}
}

// SubTabIndex returns the currently active sub-tab as a 0-based index.
func (m Model) SubTabIndex() int { return int(m.current) }

// SwitchSubTab is invoked by the sidekick when a numeric key (1-2) is
// pressed while the projects screen is active.
func (m Model) SwitchSubTab(i int) tea.Model {
	if i < 0 || i >= 2 {
		return m
	}
	m.current = tab(i)
	return m
}

// --- screener interface (sidekick) ---

// FilterActive returns true when the active sub-tab is consuming text input.
func (m Model) FilterActive() bool {
	switch st := m.subs[m.current].(type) {
	case sourceDirsModel:
		return st.filterActive()
	case worktimeProjectsModel:
		return st.filterActive()
	}
	return false
}

// StateFilter encodes the active tab + sub-tab filter for persistence.
// Format: "tab=NAME[|<sub-filter>]" — mirrors worktime/model.go shape.
func (m Model) StateFilter() string {
	tabPart := "tab=" + tabNameStr(m.current)
	sub := ""
	switch st := m.subs[m.current].(type) {
	case sourceDirsModel:
		sub = st.stateFilter()
	case worktimeProjectsModel:
		sub = st.stateFilter()
	}
	if sub != "" {
		return tabPart + "|" + sub
	}
	return tabPart
}

// StateCursor returns the active sub-tab's cursor for persistence.
func (m Model) StateCursor() int {
	switch st := m.subs[m.current].(type) {
	case sourceDirsModel:
		return st.stateCursor()
	case worktimeProjectsModel:
		return st.stateCursor()
	}
	return 0
}

// WithState restores the persisted tab + filter + cursor. Mirrors
// worktime/model.go WithState — called by the sidekick after New().
func (m Model) WithState(filter string, cursor int) tea.Model {
	subFilter := ""
	if filter != "" {
		head, rest, hasRest := strings.Cut(filter, "|")
		if rest != "" || hasRest {
			subFilter = rest
		}
		if name, ok := strings.CutPrefix(head, "tab="); ok {
			if t, ok := parseTabNameStr(name); ok {
				m.current = t
			}
		}
	}
	switch st := m.subs[m.current].(type) {
	case sourceDirsModel:
		m.subs[m.current] = st.withState(subFilter, cursor)
	case worktimeProjectsModel:
		// worktimeProjectsModel does not yet persist filter (cursor only).
		if cursor > 0 {
			st.cursor = cursor
			m.subs[m.current] = st
		}
	}
	return m
}

func tabNameStr(t tab) string {
	switch t {
	case tabSourceDirs:
		return "quellverzeichnisse"
	case tabWTProjects:
		return "worktime-projekte"
	}
	return "quellverzeichnisse"
}

func parseTabNameStr(s string) (tab, bool) {
	switch s {
	case "quellverzeichnisse":
		return tabSourceDirs, true
	case "worktime-projekte":
		return tabWTProjects, true
	}
	return tabSourceDirs, false
}

// --- helpProvider interface (sidekick) ---

// HelpSections exposes the projects-screen key bindings to the sidekick
// `?`-overlay. Updated to cover both sub-tabs.
func (Model) HelpSections() []help.Section {
	return []help.Section{
		{
			Title: "Projekte — Quellverzeichnisse",
			Keys: [][2]string{
				{"a–z (außer j/k/g/G)", "tippen → Filter direkt"},
				{"/", "Filter explizit öffnen"},
				{"j / k / ↑ / ↓", "Navigieren"},
				{"G / g", "Ende / Anfang"},
				{"Ctrl+D / Ctrl+U", "Seite vor / zurück"},
				{"Esc", "Filter löschen"},
				{"Enter", "Wechseln"},
			},
		},
		{
			Title: "Projekte — Worktime-Projekte",
			Keys: [][2]string{
				{"j / k", "Navigieren"},
				{"n", "Neues Projekt"},
				{"r", "Umbenennen"},
				{"a", "Archivieren"},
				{"A", "Archivierte anzeigen/verstecken"},
				{"/", "Filter"},
				{"Enter", "Zu Worktime wechseln"},
			},
		},
	}
}

// --- tea.Model ---

// Init starts both sub-models.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.subs {
		if cmd := s.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Update routes messages to the active sub-model and handles the
// global tab-switching keys (1/2/Tab).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var cmds []tea.Cmd
		for i, s := range m.subs {
			updated, cmd := s.Update(msg)
			m.subs[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)
	}

	// Async messages — fan out to active sub-model only (and the inactive
	// one for size/reload messages). To keep things simple: fan to all.
	var cmds []tea.Cmd
	for i, s := range m.subs {
		updated, cmd := s.Update(msg)
		m.subs[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// When the active sub-tab consumes text input, forward directly.
	if m.FilterActive() {
		return m.forwardToCurrent(msg)
	}
	// Tab-router: 1/2/Tab switch sub-tabs when no filter is active.
	if next, ok := m.handleTabRouterKey(msg); ok {
		return next, nil
	}
	return m.forwardToCurrent(msg)
}

func (m Model) handleTabRouterKey(msg tea.KeyPressMsg) (Model, bool) {
	switch msg.String() {
	case "1":
		m.current = tabSourceDirs
		return m, true
	case "2":
		m.current = tabWTProjects
		return m, true
	case "tab":
		m.current = (m.current + 1) % 2
		return m, true
	}
	return Model{}, false
}

func (m Model) forwardToCurrent(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.subs[m.current].Update(msg)
	m.subs[m.current] = updated
	return m, cmd
}

// View renders the active sub-tab. Each sub-tab renders its own
// titlebox with the tab-strip as title (same pattern as worktime).
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.width == 0 {
		return ""
	}
	// Render the active sub-tab's content, replacing its titlebox title
	// with the shared tab-strip so both tabs are always visible.
	//
	// Each sub-model's viewContent() already wraps with titlebox.Render;
	// we re-wrap here by: (a) extracting the sub-model body raw content
	// and (b) calling titlebox.Render with the tab-strip title.
	// But that would duplicate the sub-model's own box chrome.
	//
	// Simpler approach that mirrors worktime: the sub-models call
	// titlebox.Render themselves with their own title (list count, etc.),
	// and the root model just returns the sub-model's viewContent()
	// unchanged — with the sub-tab strip INSIDE the sub-model's titlebox
	// title, not at the root level.
	//
	// To keep both strips visible, the sub-models pass tabStrip(width)
	// as their title. We achieve this by letting the root call the sub-
	// model render function with an injected tab-strip title. Since the
	// sub-models are private types the root can call their titlebox render
	// directly.
	stripTitle := m.tabStrip(m.width)
	switch st := m.subs[m.current].(type) {
	case sourceDirsModel:
		return st.viewContentWithTitle(stripTitle)
	case worktimeProjectsModel:
		return st.viewContentWithTitle(stripTitle)
	}
	return m.subs[m.current].View().Content
}

// tabStrip renders the two-tab navigation as the titlebox title string.
// Three-step degradation for narrow panes: full labels → short labels →
// single chars. Budget = width - 6 (titlebox chrome: "╭─ " + " " + "╮").
func (m Model) tabStrip(width int) string {
	labels := m.SubTabs()
	short := []string{"Quell.", "WT-Proj."}
	single := []string{"Q", "W"}
	budget := width - 6
	if budget < 1 {
		budget = 1
	}
	for _, opt := range []struct {
		labels []string
		sep    string
	}{
		{labels, "  ·  "},
		{short, "  ·  "},
		{short, " · "},
		{single, " · "},
	} {
		if out := m.renderTabs(opt.labels, opt.sep); lipgloss.Width(out) <= budget {
			return out
		}
	}
	return m.renderTabs(single, " ")
}

func (m Model) renderTabs(labels []string, sep string) string {
	activeStyle := lipgloss.NewStyle().
		Foreground(m.pal.Sem().Accent).
		Bold(true).
		Underline(true)
	out := ""
	for i, l := range labels {
		if i > 0 {
			out += theme.Dim(sep, m.pal)
		}
		if tab(i) == m.current {
			out += activeStyle.Render(l)
		} else {
			out += theme.Dim(l, m.pal)
		}
	}
	return out
}
