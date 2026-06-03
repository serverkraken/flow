// Package projects — source_dirs.go
//
// sourceDirsModel is the "Quellverzeichnisse" sub-tab: the existing fuzzy-
// filterable SourceDir listing that was previously the top-level Model.
// Promoted to a sub-model by Task 18; the public Model is now the host.
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
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/usecase"
)

// sourceDirsLoadedMsg carries the result of the async SourceDir enumeration.
type sourceDirsLoadedMsg struct {
	projects []domain.SourceDir
	err      error
}

// sourceDirsSwitchedMsg carries the result of a tmux session switch.
type sourceDirsSwitchedMsg struct {
	err error
}

// sourceDirsStyles caches the palette-derived lipgloss styles used by
// the SourceDir row renderer.
type sourceDirsStyles struct {
	border lipgloss.Style // Sem.Border — filter separator rule
	marker lipgloss.Style // Sem.Active — tmux-session hint glyph
}

func newSourceDirsStyles(p theme.Palette) sourceDirsStyles {
	sem := p.Sem()
	return sourceDirsStyles{
		border: lipgloss.NewStyle().Foreground(sem.Border),
		marker: lipgloss.NewStyle().Foreground(sem.Active),
	}
}

// sourceDirsModel is the Quellverzeichnisse sub-tab: fuzzy-filterable
// list of SourceDir rows. Was previously the top-level Model; demoted
// by Task 18 to make room for the Worktime-Projekte sub-tab.
type sourceDirsModel struct {
	all        []domain.SourceDir
	visible    []domain.SourceDir
	highlights [][]int
	cursor     int
	offset     int
	filter     textinput.Model
	pal        theme.Palette
	styles     sourceDirsStyles
	width      int
	height     int
	err        error
	loading    bool
	rootDir    string

	switchToast *toast.Model

	reader   *usecase.ProjectsReader
	switcher *usecase.ProjectSwitcher

	mode Mode
}

func newSourceDirs(p theme.Palette, rootDir string, reader *usecase.ProjectsReader, switcher *usecase.ProjectSwitcher, mode Mode) sourceDirsModel {
	ti := form.NewTextInput("filter…", p)
	return sourceDirsModel{
		pal:      p,
		styles:   newSourceDirsStyles(p),
		filter:   ti,
		rootDir:  rootDir,
		loading:  true,
		reader:   reader,
		switcher: switcher,
		mode:     mode,
	}
}

func (s sourceDirsModel) Init() tea.Cmd {
	r := s.reader
	return func() tea.Msg {
		ps, err := r.List()
		return sourceDirsLoadedMsg{projects: ps, err: err}
	}
}

func (s sourceDirsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		return s, nil

	case sourceDirsLoadedMsg:
		s.loading = false
		s.err = msg.err
		s.all = msg.projects
		s.applyFilter()
		return s, nil

	case sourceDirsSwitchedMsg:
		if msg.err != nil {
			t := toast.NewDanger("Aktion fehlgeschlagen: "+msg.err.Error(), s.pal)
			s.switchToast = &t
			return s, t.Init()
		}
		return s, tea.Quit

	case toast.DismissedMsg:
		s.switchToast = nil
		return s, nil

	case tea.KeyPressMsg:
		if s.filter.Focused() {
			return s.handleFilterKey(msg)
		}
		return s.handleNormalKey(msg)
	}
	return s, nil
}

func (s sourceDirsModel) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		s.filter.Focus()
		return s, textinput.Blink
	case "j", "down":
		if s.cursor < len(s.visible)-1 {
			s.cursor++
			s.ensureCursorVisible()
		}
		return s, nil
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
			s.ensureCursorVisible()
		}
		return s, nil
	case "G":
		s.cursor = max(0, len(s.visible)-1)
		s.ensureCursorVisible()
		return s, nil
	case "g":
		s.cursor = 0
		s.ensureCursorVisible()
		return s, nil
	case "pgdown", "ctrl+d":
		s.cursor = min(len(s.visible)-1, s.cursor+s.maxVisible())
		s.ensureCursorVisible()
		return s, nil
	case "pgup", "ctrl+u":
		s.cursor = max(0, s.cursor-s.maxVisible())
		s.ensureCursorVisible()
		return s, nil
	case "enter":
		if len(s.visible) > 0 {
			return s, s.switchToProject(s.visible[s.cursor])
		}
		return s, nil
	}

	// Type-to-filter: any other single printable character auto-focuses
	// the filter and routes the keystroke into it.
	str := msg.String()
	if len(str) == 1 && str[0] >= ' ' && str[0] < 127 {
		s.filter.Focus()
		var cmd tea.Cmd
		prev := s.filter.Value()
		s.filter, cmd = s.filter.Update(msg)
		if s.filter.Value() != prev {
			s.applyFilter()
		}
		return s, tea.Batch(cmd, textinput.Blink)
	}
	return s, nil
}

func (s sourceDirsModel) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.filter.Blur()
		s.filter.SetValue("")
		s.applyFilter()
		return s, nil
	case "enter":
		s.filter.Blur()
		if len(s.visible) > 0 {
			return s, s.switchToProject(s.visible[s.cursor])
		}
		return s, nil
	}
	var cmd tea.Cmd
	prev := s.filter.Value()
	s.filter, cmd = s.filter.Update(msg)
	if s.filter.Value() != prev {
		s.applyFilter()
	}
	return s, cmd
}

func (s *sourceDirsModel) applyFilter() {
	q := s.filter.Value()
	if q == "" {
		s.visible = s.all
		s.highlights = make([][]int, len(s.visible))
	} else {
		names := make([]string, len(s.all))
		for i, p := range s.all {
			names[i] = p.Name
		}
		matches := fuzzy.Find(q, names)
		s.visible = make([]domain.SourceDir, len(matches))
		s.highlights = make([][]int, len(matches))
		for i, match := range matches {
			s.visible[i] = s.all[match.Index]
			s.highlights[i] = match.MatchedIndexes
		}
	}
	if s.cursor >= len(s.visible) {
		s.cursor = max(0, len(s.visible)-1)
	}
	s.offset = 0
	s.ensureCursorVisible()
}

func (s sourceDirsModel) maxVisible() int {
	return max(1, s.height-theme.PickerChromeRows)
}

func (s *sourceDirsModel) ensureCursorVisible() {
	vis := s.maxVisible()
	if s.cursor < s.offset {
		s.offset = s.cursor
	} else if s.cursor >= s.offset+vis {
		s.offset = s.cursor - vis + 1
	}
}

func (s sourceDirsModel) switchToProject(p domain.SourceDir) tea.Cmd {
	sw := s.switcher
	return func() tea.Msg {
		return sourceDirsSwitchedMsg{err: sw.Switch(p)}
	}
}

// filterActive reports whether the filter input is currently focused.
func (s sourceDirsModel) filterActive() bool { return s.filter.Focused() }

// stateFilter returns the current filter value for persistence.
func (s sourceDirsModel) stateFilter() string { return s.filter.Value() }

// stateCursor returns the cursor position for persistence.
func (s sourceDirsModel) stateCursor() int { return s.cursor }

// withState restores filter and cursor from persisted state.
func (s sourceDirsModel) withState(filter string, cursor int) sourceDirsModel {
	s.filter.SetValue(filter)
	s.cursor = cursor
	return s
}

// View renders the Quellverzeichnisse tab body (no outer titlebox — the
// host wraps with its tab-strip title).
func (s sourceDirsModel) View() tea.View {
	return tea.NewView(s.viewContent())
}

func (s sourceDirsModel) viewContent() string {
	if s.width == 0 {
		return ""
	}
	inner := s.width - 4

	var rows []string
	prompt := theme.Dim(glyphs.Info+" ", s.pal)
	if s.filter.Focused() {
		prompt = theme.Heading(glyphs.Active+" ", s.pal)
	}
	rows = append(rows, prompt+s.filter.View())
	rows = append(rows, s.styles.border.Render(strings.Repeat("─", inner)))

	switch {
	case s.loading:
		rows = append(rows, theme.Dim("  lade Quellverzeichnisse…", s.pal))
	case s.err != nil:
		rows = append(rows, theme.Err("  "+s.err.Error(), s.pal))
	case len(s.all) == 0:
		rows = append(rows, theme.Dim("  keine Projekte gefunden — $SOURCECODE_ROOT prüfen", s.pal))
	case len(s.visible) == 0:
		rows = append(rows, s.renderEmptyState()...)
	default:
		vis := s.maxVisible()
		end := min(s.offset+vis, len(s.visible))
		if s.offset > 0 {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d vorherige…", glyphs.Up, s.offset), s.pal))
		}
		for i := s.offset; i < end; i++ {
			rows = append(rows, s.renderRow(i == s.cursor, s.visible[i], s.highlights[i], inner))
		}
		if end < len(s.visible) {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d weitere…", glyphs.Down, len(s.visible)-end), s.pal))
		}
	}

	body := strings.Join(rows, "\n")
	label := lastSegment(s.rootDir)
	var title string
	if s.filter.Value() != "" {
		title = fmt.Sprintf("Projekte · %s · %d/%d", label, len(s.visible), len(s.all))
	} else {
		title = fmt.Sprintf("Projekte · %s · %d", label, len(s.all))
	}
	box := titlebox.Render(title, body, s.width, s.pal)
	hints := strings.Join([]string{
		"Enter → wechseln",
		"j/k → bewegen",
		uistrings.HintFilter,
		uistrings.HintHelp,
	}, "  ·  ")
	footer := statusbar.Hints(hints, s.pal)
	return box + "\n" + toast.SlotLine(s.switchToast, "  ") + "\n" + footer
}

func (s sourceDirsModel) renderRow(selected bool, p domain.SourceDir, highlight []int, width int) string {
	hint := ""
	if p.HasTmuxSession {
		hint = s.styles.marker.Render(glyphs.Active)
	}
	return picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected:      selected,
		Label:         p.Name,
		Hint:          hint,
		Width:         width,
		Match:         highlight,
		HintPreStyled: true,
	}, s.pal)
}

func (s sourceDirsModel) renderEmptyState() []string {
	return []string{
		"",
		theme.Dim("  keine Treffer für »"+s.filter.Value()+"«", s.pal),
		"",
		theme.Dim("  "+uistrings.HintClearFilter, s.pal),
	}
}

// viewContentWithTitle renders the same content as viewContent but uses an
// externally supplied titlebox title (the host's tab-strip). Called by the
// root Model so both sub-tabs appear in the titlebox header.
func (s sourceDirsModel) viewContentWithTitle(title string) string {
	if s.width == 0 {
		return ""
	}
	inner := s.width - 4

	var rows []string
	prompt := theme.Dim(glyphs.Info+" ", s.pal)
	if s.filter.Focused() {
		prompt = theme.Heading(glyphs.Active+" ", s.pal)
	}
	rows = append(rows, prompt+s.filter.View())
	rows = append(rows, s.styles.border.Render(strings.Repeat("─", inner)))

	switch {
	case s.loading:
		rows = append(rows, theme.Dim("  lade Quellverzeichnisse…", s.pal))
	case s.err != nil:
		rows = append(rows, theme.Err("  "+s.err.Error(), s.pal))
	case len(s.all) == 0:
		rows = append(rows, theme.Dim("  keine Projekte gefunden — $SOURCECODE_ROOT prüfen", s.pal))
	case len(s.visible) == 0:
		rows = append(rows, s.renderEmptyState()...)
	default:
		vis := s.maxVisible()
		end := min(s.offset+vis, len(s.visible))
		if s.offset > 0 {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d vorherige…", glyphs.Up, s.offset), s.pal))
		}
		for i := s.offset; i < end; i++ {
			rows = append(rows, s.renderRow(i == s.cursor, s.visible[i], s.highlights[i], inner))
		}
		if end < len(s.visible) {
			rows = append(rows, theme.Dim(fmt.Sprintf("  %s %d weitere…", glyphs.Down, len(s.visible)-end), s.pal))
		}
	}

	body := strings.Join(rows, "\n")
	box := titlebox.Render(title, body, s.width, s.pal)
	hints := strings.Join([]string{
		"Enter → wechseln",
		"j/k → bewegen",
		uistrings.HintFilter,
		uistrings.HintHelp,
	}, "  ·  ")
	footer := statusbar.Hints(hints, s.pal)
	return box + "\n" + toast.SlotLine(s.switchToast, "  ") + "\n" + footer
}

// lastSegment returns the last path component (after the final "/").
func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
