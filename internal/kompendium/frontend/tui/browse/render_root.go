package browse

// Browse rendering root — View komponiert Header, Body, Footer und
// Status; renderHeader (mit Type-Counts, Repo-Chip, Separator,
// Status-Line, Search-Segment) und renderBody/renderListPane/
// renderPaginator wohnen hier mit. Row-Rendering, Status-Bar,
// Footer-Hints und Modal-Overlays liegen weiter in render_row.go,
// render_status.go bzw. render_modal.go. Split aus model.go (Skill
// §No-Monoliths).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// View renders the current model as a string.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.quitting {
		return ""
	}
	if m.mode == ModeView {
		return m.viewer.View()
	}
	if m.mode == ModeWritePicker {
		// Picker manages its own width/height + center placement, so it
		// gets the full screen as a passthrough — no frameContent wrap
		// (which would double-border it).
		return m.picker.View()
	}
	if m.loadErr != nil {
		return frameContent(m.width, m.height, errorStyle.Render("Fehler: "+m.loadErr.Error()))
	}
	if !m.loaded {
		loading := lipgloss.JoinHorizontal(lipgloss.Center, m.spin.View(), " ", dimStyle.Render("lädt…"))
		return frameContent(m.width, m.height, loading)
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()
	statusBar := m.renderStatusBar()

	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer, statusBar)
	base := frameContent(m.width, m.height, content)

	switch {
	case m.showHelp:
		return overlay(base, m.renderHelpOverlay(), m.width, m.height)
	case m.mode == ModeConfirmDelete:
		return overlay(base, m.renderDeleteModal(), m.width, m.height)
	}
	return base
}

// renderHeader is the top status block: kompendium counts (with per-type
// breakdown and repo context), separator, filter, search, and any inline
// error. The headline is rendered as a single styled string (no
// padding-injecting badges) so test asserts on the contiguous
// "kompendium — N/M notes" substring still match.
func (m Model) renderHeader() string {
	headline := headlineStyle.Render(
		fmt.Sprintf("kompendium — %d/%d Notizen", len(m.visible), len(m.all)),
	)

	headlineRow := []string{headline}
	if breakdown := m.renderTypeCounts(); breakdown != "" {
		headlineRow = append(headlineRow, statusLineStyle.Render("  ·  "), breakdown)
	}
	if repo := m.renderRepoChip(); repo != "" {
		headlineRow = append(headlineRow, "  ", repo)
	}
	headlineLine := lipgloss.JoinHorizontal(lipgloss.Top, headlineRow...)

	separator := m.renderSeparator()

	statusLine := m.renderStatusLine()

	headerBlock := lipgloss.JoinVertical(lipgloss.Left, headlineLine, separator, statusLine)
	if m.editErr != nil {
		headerBlock = lipgloss.JoinVertical(lipgloss.Left, headerBlock,
			errorStyle.Render("Fehler beim Bearbeiten: "+m.editErr.Error()))
	}
	return headerBlock
}

// renderTypeCounts emits the per-type breakdown shown next to the
// headline (e.g. "●3 daily  ●1 proj  ●0 free"). Counts reflect the
// currently visible (filtered + searched) set so the user can see what
// just dropped out of view.
func (m Model) renderTypeCounts() string {
	if len(m.all) == 0 {
		return ""
	}
	var d, p, f int
	for _, e := range m.visible {
		switch e.Meta.Type {
		case domain.TypeDaily:
			d++
		case domain.TypeProject:
			p++
		case domain.TypeFree:
			f++
		}
	}
	parts := []string{
		countDailyStyle.Render(fmt.Sprintf(glyphs.CountDaily+" %d", d)) + dimStyle.Render(" täglich"),
		countProjectStyle.Render(fmt.Sprintf(glyphs.CountProject+" %d", p)) + dimStyle.Render(" projekt"),
		countFreeStyle.Render(fmt.Sprintf(glyphs.CountFree+" %d", f)) + dimStyle.Render(" frei"),
	}
	return strings.Join(parts, "  ")
}

// renderRepoChip shows the current repo as a small pill when running
// inside a project. Empty when not in a repo.
func (m Model) renderRepoChip() string {
	if m.currentRepo == "" {
		return ""
	}
	return repoChipStyle.Render(shortProjectLabel(string(m.currentRepo)))
}

// renderSeparator draws a soft horizontal rule under the headline. Width
// is the inner content width — falls back to a sane minimum when the
// model hasn't received its first WindowSizeMsg yet.
func (m Model) renderSeparator() string {
	w := m.contentWidth()
	if w <= 0 {
		w = 60
	}
	return headerSeparatorStyle.Render(strings.Repeat("─", w))
}

// renderStatusLine renders the second header row: type filter label und
// die (optional umrandete) Suchleiste. UX-Review §4.4 + §1.8: vorher
// hieß das Label „Filter:" und wurde immer gezeigt — auch wenn weder
// Type-Filter noch Suche aktiv waren, was den Drei-Doppelpunkt-Mismatch
// „Filter:  · Suche: …" produzierte. Jetzt: Label „Typ:" (das ist der
// type-cycle täglich/projekt/frei, kein Text-Filter) und der ganze
// Eintrag wird suppressed, wenn der Type-Filter leer ist.
func (m Model) renderStatusLine() string {
	var parts []string
	if label := m.filter.label(); label != "" {
		parts = append(parts, statusKeyStyle.Render("Typ:")+" "+statusValueStyle.Render(label))
	}
	if search := m.renderSearchSegment(); search != "" {
		parts = append(parts, search)
	}
	return strings.Join(parts, statusLineStyle.Render("  ·  "))
}

// renderSearchSegment is the inline search affordance: nothing when
// neither active nor populated, a passive label when the user has a
// stashed query, a yellow label + raw textinput view when in ModeSearch.
//
// The textinput view already carries its own ANSI sequences (cursor in
// reverse + colored), so we MUST NOT pipe it through another lipgloss
// style with Bold/Underline — lipgloss's ansi wrapper mangles nested
// sequences and the raw escape codes leak as visible text. Yellow label
// alone is the focus cue.
func (m Model) renderSearchSegment() string {
	if m.mode == ModeSearch {
		view := m.search.View()
		if view == "" {
			view = glyphs.AccentBar
		}
		return searchActiveLabelStyle.Render("Suche:") + " " + view
	}
	if v := m.search.Value(); v != "" {
		return searchPassiveLabelStyle.Render("Suche:") + " " + searchValueStyle.Render(v)
	}
	return ""
}

// renderBody returns the list pane (and preview pane in two-pane layout).
func (m Model) renderBody() string {
	listPane := m.renderListPane()
	if !m.twoPane() {
		return listPane
	}
	previewPane := m.renderPreviewPane()
	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, "  ", previewPane)
}

// renderListPane returns the list block. There is intentionally no inner
// border — the outer rounded frame already provides a chrome, and a
// second nested border looked broken whenever a row overflowed and the
// border characters fell out of alignment.
//
// The paginator slot is *always* reserved (one line, blank when there
// are too few entries to need dots) so the pane's overall height stays
// constant across short and long lists. Variable chrome would push the
// total content past the frame's interior height and corrupt the
// bottom border.
func (m Model) renderListPane() string {
	w := m.listPaneWidth()
	// Im Zwei-Pane-Layout trägt die Liste den Fokus-Titel (Fg+Bold), die
	// Vorschau bleibt muted — so liest sich, welche Pane interaktiv ist
	// (UX-Review L3). Single-Pane braucht den Cue nicht.
	titleStyle := panelTitleStyle
	if m.twoPane() {
		titleStyle = panelTitleFocusStyle
	}
	header := titleStyle.Render("notizen")
	if len(m.visible) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.renderEmptyState(w), "")
	}
	rows := m.renderListRows(w)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", rows, m.renderPaginator())
}

// renderPaginator returns a dotted page indicator plus a "X/N" counter
// for the cursor's position. Empty when there are five or fewer entries
// — at that scale the dots are noise and the entire list is visible
// anyway. PerPage is rounded so we never render more than ~12 dots,
// otherwise long notebooks turn the indicator into a wall.
func (m Model) renderPaginator() string {
	const maxDots = 12
	const minForDots = 6
	if len(m.visible) < minForDots {
		return ""
	}
	perPage := (len(m.visible) + maxDots - 1) / maxDots
	if perPage < 1 {
		perPage = 1
	}
	p := m.pager
	p.PerPage = perPage
	p.SetTotalPages(len(m.visible))
	p.Page = m.cursor / perPage
	dots := p.View()
	counter := paginatorCounterStyle.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.visible)))
	return dots + "  " + counter
}
