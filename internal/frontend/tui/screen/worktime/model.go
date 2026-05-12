// Package worktime is the multi-tab worktime screen — port-driven
// successor to internal/screen/worktime.
//
// The root Model holds four sub-models (Heute / Woche / History /
// Frei) in a fixed-index array, with tab switching, an adaptive ticker
// (1 s for the first minute of an active session, then 10 s) and a
// lightweight dayRefreshMsg. The four sub-models live in their own
// files: today.go (Heute), week.go (Woche), history.go (History),
// dayoffs.go (Frei).
package worktime

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/components/titlebox"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// stateRestorer is the optional contract a sub-model implements when
// it can restore its (filter, cursor) state from persistence. Mirrors
// sidekick's stateRestorer shape — duplicated so this package stays
// self-contained.
type stateRestorer interface {
	WithState(filter string, cursor int) tea.Model
}

// Deps bundles every use case the worktime screen and its sub-models
// consume. Wired by the composition root and threaded into all four
// sub-models so they never reach for I/O directly.
type Deps struct {
	Reader        *usecase.WorktimeReader
	Stats         *usecase.StatsComputer
	SessionWriter *usecase.SessionWriter
	Tagger        *usecase.Tagger
	DayOffStore   ports.DayOffStore
	DayOffWriter  *usecase.DayOffWriter
	LinkReader    *usecase.LinkReader
	LinkWriter    *usecase.LinkWriter
	Reporter      *usecase.Reporter
	NoteOpener    *usecase.NoteOpener
	// NoteLister füttert den Note-Attach-Picker in Heute mit den
	// jüngsten Kompendium-Notes. Optional — bei nil degradiert der
	// Dialog zur reinen ID-Eingabe (Pre-Picker-Verhalten).
	NoteLister NoteLister
	// NoteReader liest den Markdown-Body einer Note für den
	// integrierten Inline-Viewer (Heute `o`-Key). Composition-Root
	// muss ihn verdrahten; nil produziert einen klaren Error-Toast
	// statt eines stillen Workarounds.
	NoteReader NoteReader
	// MarkdownRenderer rendert den Note-Body inline. Geteilt mit dem
	// Cheatsheet-Screen — gleiche Pipeline, gleiches Styling. Optional;
	// bei nil zeigt der Viewer den Raw-Markdown.
	MarkdownRenderer ports.MarkdownRenderer
	Clock            interface{ Now() time.Time }
	// Output is the worktime menu's three-target sink (Clipboard /
	// tmux-Split / Datei in ~/Downloads). Wired in cmd/flow/main.go via
	// internal/adapter/output. Slice B: nil-tolerant (no flow uses it
	// yet); Slice C/D/E start dispatching Brief / Export / Stats output
	// through this port.
	Output ports.Output
	// HomeDir is the absolute path to the user's home directory. Used
	// by the menu's "saved as ~/Downloads/…" toast for path-shortening.
	// Pre-A1 the screen called os.Getenv("HOME") directly.
	HomeDir string
	// Land is the dayoff Bundesland default ($WORKTIME_LAND in
	// production). Resolved by the composition root so the screen and
	// its menu_actions don't reach for env vars directly. Empty string
	// falls back to "NW" inside the helpers.
	Land string
}

// tab identifies one of the four worktime sub-screens.
type tab int

const (
	tabHeute   tab = 0
	tabWoche   tab = 1
	tabHistory tab = 2
	tabFrei    tab = 3
)

// Internal messages.

// tickMsg drives the adaptive ticker. The interval is
// fast (1 s) for the first minute of an active session, then slow
// (10 s) so a long-running tracker doesn't burn CPU.
type tickMsg time.Time

// dayRefreshMsg is the lightweight per-tick day reload — only the
// today snapshot is reloaded, not the heavier weekly / history /
// kompendium calls.
type dayRefreshMsg struct{}

const (
	tickFast = 1 * time.Second
	tickSlow = 10 * time.Second
)

// Model is the root bubbletea model for the worktime screen.
type Model struct {
	pal     theme.Palette
	deps    Deps
	width   int
	height  int
	current tab
	subs    [4]tea.Model
	menu    menuModel
	// brief — optionaler Fullscreen-Overlay für den integrierten
	// Brief-Viewer (Menu → Brief Week/Month → Target tmux-Split).
	// Bei nil läuft der reguläre Tab-Body; sonst übernimmt brief
	// Input + Render bis ein ExitMsg vom Overlay zurückkommt.
	brief *markdown_overlay.Model
}

// New constructs the worktime root model with the four sub-models
// wired against the given Deps.
func New(p theme.Palette, deps Deps) Model {
	return Model{
		pal:  p,
		deps: deps,
		subs: [4]tea.Model{
			newHeute(p, deps),
			newWoche(p, deps),
			newHistory(p, deps),
			newFrei(p, deps),
		},
		menu: newMenuModel(p, deps),
	}
}

// WithState restores the persisted tab selection (parsed from filter,
// shape "tab=NAME[|sub-filter]") and forwards the persisted cursor +
// the sub-filter half to the active sub-model when that sub-model
// supports state restoration. Called by the sidekick root after
// constructing the model.
func (m Model) WithState(filter string, cursor int) tea.Model {
	subFilter := ""
	if filter != "" {
		head, rest, hasRest := strings.Cut(filter, "|")
		if rest != "" || hasRest {
			subFilter = rest
		}
		if name, ok := strings.CutPrefix(head, "tab="); ok {
			if t, ok := parseTabName(name); ok {
				m.current = t
			}
		}
	}
	if sr, ok := m.subs[m.current].(stateRestorer); ok {
		m.subs[m.current] = sr.WithState(subFilter, cursor)
	}
	return m
}

// FilterActive returns whether either the action menu, the brief
// overlay, or the active sub-model is currently consuming text input.
// The Worktime root, sidekick parent, and tab-switching keys all check
// this before claiming letter keys back.
func (m Model) FilterActive() bool {
	if m.brief != nil {
		return true
	}
	if m.menu.Active() {
		return true
	}
	if fa, ok := m.subs[m.current].(filterActiver); ok {
		return fa.FilterActive()
	}
	return false
}

// StateFilter returns the persisted state for the worktime screen.
// Encodes the active tab as "tab=N" plus, when the active sub-model
// itself carries a filter, the sub-model's own filter via "tab=N|<f>".
// WithState parses this back into (tab, filter) for restoration.
func (m Model) StateFilter() string {
	tabPart := "tab=" + tabName(m.current)
	if fa, ok := m.subs[m.current].(filterActiver); ok {
		if f := fa.StateFilter(); f != "" {
			return tabPart + "|" + f
		}
	}
	return tabPart
}

// tabName returns a stable string identifier for t — used by
// StateFilter so persisted state survives a tab-index renumbering.
func tabName(t tab) string {
	switch t {
	case tabHeute:
		return "heute"
	case tabWoche:
		return "woche"
	case tabHistory:
		return "history"
	case tabFrei:
		return "frei"
	}
	return "heute"
}

// parseTabName is the inverse of tabName.
func parseTabName(s string) (tab, bool) {
	switch s {
	case "heute":
		return tabHeute, true
	case "woche":
		return tabWoche, true
	case "history":
		return tabHistory, true
	case "frei":
		return tabFrei, true
	}
	return tabHeute, false
}

// StateCursor returns the active sub-model's cursor for state
// persistence. Each tab persists its own cursor shape (Heute's row,
// Woche's day, History's drill index, Frei's row). The active tab is
// encoded in StateFilter so WithState can restore both halves.
func (m Model) StateCursor() int {
	if cs, ok := m.subs[m.current].(cursorStater); ok {
		return cs.StateCursor()
	}
	return 0
}

// HandlesBack reports whether the worktime screen wants to consume the
// global `b` key. It always returns false now, so `b` from any
// worktime tab falls through to the sidekick and switches to Palette.
//
// The previous behaviour cycled tabs backward inside Worktime when the
// user wasn't on Heute — which meant `b` had two different meanings on
// different tabs (jump-to-Palette on Heute, cycle-tabs elsewhere). The
// review found that inconsistent and surprising; users expect `b` to
// be a global "back to launcher". Tab cycling stays available via
// 1/2/3/4 and Tab; the lost backward-cycle had no dedicated keybinding
// and is not missed.
func (m Model) HandlesBack() bool { return false }

// ConsumesKeys lists letter / punctuation keys the active sub-model and
// the worktime-root menu claim, so the sidekick's global navigation
// (p / n / f / w / c) doesn't intercept keys the worktime surface itself
// binds. `:` is always claimed because the action menu lives at the
// root and must open from any tab.
func (m Model) ConsumesKeys() []string {
	keys := []string{":"}
	if kc, ok := m.subs[m.current].(interface{ ConsumesKeys() []string }); ok {
		keys = append(keys, kc.ConsumesKeys()...)
	}
	return keys
}

// Init starts every sub-model concurrently and schedules the first
// tick.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.scheduleTick()}
	for _, s := range m.subs {
		if cmd := s.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Update routes messages to the active sub-model and handles the
// global tab keys + tick. When the action menu is open, all keys go
// to the menu and tab-switching is suspended.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case briefViewMsg:
		// Brief-Result aus dem Menu (briefCmd → outputTargetSplit).
		// Menu schließt sich, Brief-Overlay übernimmt; q/Esc/b dort
		// emittieren markdown_overlay.ExitMsg, der Worktime-Root
		// verwirft den Overlay und das Worktime-Tab nimmt zurück.
		m.menu = m.menu.closeMenu()
		bv := newBriefView(msg.title, msg.body, m.width, m.height, m.deps)
		m.brief = &bv
		return m, nil

	case markdown_overlay.ExitMsg:
		// Schließsignal aus dem brief- ODER note-Overlay. Hier nur die
		// brief-Seite; der note-Overlay sitzt im heute-Submodel und
		// behandelt das Signal lokal in today.go.
		if m.brief != nil {
			m.brief = nil
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu = m.menu.SetSize(msg.Width, msg.Height)
		if m.brief != nil {
			bv := m.brief.SetSize(msg.Width, msg.Height)
			m.brief = &bv
		}
		var cmds []tea.Cmd
		for i, s := range m.subs {
			updated, cmd := s.Update(msg)
			m.subs[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tickMsg:
		// Forward the tick to the active sub-model as a dayRefreshMsg —
		// the heavy snapshot reloads stay scoped to the visible tab.
		var cmds []tea.Cmd
		updated, cmd := m.subs[m.current].Update(dayRefreshMsg{})
		m.subs[m.current] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, m.scheduleTick())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Async messages (loadedMsg variants from sub-models, toast
	// dismiss ticks for the menu) are dispatched to every sub-screen
	// PLUS the menu so each picks up the ones it owns. Recipients
	// drop messages they don't recognise.
	var cmds []tea.Cmd
	for i, s := range m.subs {
		updated, cmd := s.Update(msg)
		m.subs[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.menu.Active() {
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// handleKeyMsg dispatches a key when no async / tick / window message
// is in flight. Order:
//  1. q is the universal exit key — returns tea.Quit from any sub-
//     mode (menu, dialog, picker, help overlay) UNLESS a textinput is
//     currently focused; in that case 'q' is a literal letter the user
//     wants in their tag / note / range / HH:MM input.
//  2. Menu owns input while open.
//  3. Tab-router keys (1/2/3/4/Tab/b/`:`) when no dialog/menu blocks.
//  4. Forward everything else to the active sub-model.
//
// Split off Update to keep cyclomatic complexity inside the project
// budget.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Brief-Overlay claimt alle Tasten zuerst. q im Overlay schließt
	// den Overlay (kein Quit) — der User würde sonst aus Versehen den
	// ganzen Sidekick verlieren, nur weil er den Brief schließen will.
	// Das Schließsignal kommt als markdown_overlay.ExitMsg im
	// Top-Level Update-Switch zurück.
	if m.brief != nil {
		next, cmd := m.brief.Update(msg)
		m.brief = &next
		return m, cmd
	}
	if msg.String() == "q" && !m.textInputActive() {
		return m, tea.Quit
	}
	if m.menu.Active() {
		updated, cmd := m.menu.Update(msg)
		m.menu = updated
		return m, cmd
	}
	if !m.FilterActive() {
		if next, ok := m.handleTabRouterKey(msg); ok {
			return next, nil
		}
	}
	updated, cmd := m.subs[m.current].Update(msg)
	m.subs[m.current] = updated
	return m, cmd
}

// handleTabRouterKey handles the global tab-switching keys plus the
// `:` action-menu trigger. Returns (model, true) when the key was
// claimed; (zero, false) lets the caller forward the key to the active
// sub-model.
func (m Model) handleTabRouterKey(msg tea.KeyMsg) (Model, bool) {
	switch msg.String() {
	case ":":
		m.menu = m.menu.openMenu(m.current)
		return m, true
	case "1":
		m.current = tabHeute
		return m, true
	case "2":
		m.current = tabWoche
		return m, true
	case "3":
		m.current = tabHistory
		return m, true
	case "4":
		m.current = tabFrei
		return m, true
	case "tab":
		m.current = (m.current + 1) % 4
		return m, true
	}
	// `b` ist jetzt global — die sidekick-Layer routet ihn zu Palette.
	// Worktime claimt `b` nicht mehr (siehe HandlesBack-Comment).
	return Model{}, false
}

// View renders the active sub-model with a tab strip on top. When the
// action menu is open it replaces the tab body — the tab strip stays
// so the user keeps the visual anchor of which tab they came from.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	// Brief-Overlay ersetzt das Worktime-Outer komplett — kein Tab-
	// Strip, keine titlebox-Umhüllung außenrum. Der Overlay bringt
	// seine eigene Box + Footer + StatusBar mit (markdown_overlay).
	if m.brief != nil {
		return m.brief.View()
	}
	var body string
	if m.menu.Active() {
		body = m.menu.View()
	} else {
		body = m.subs[m.current].View()
		if body == "" {
			body = theme.Dim("  (lädt …)", m.pal)
		}
	}
	return titlebox.Render(m.tabStrip(m.width), body, m.width, m.pal)
}

// tabStrip renders the four-tab navigation. Three-step degradation keeps
// the strip inside the titlebox budget on narrow tmux panes: full labels
// with "  ·  " spacing → compact "·" separators → single-char fallback
// ("H · W · Hi · F"). titlebox.Render reserves "╭─ " (3) + " " (1) +
// "╮" (1) + ≥1 right-dash = 6 chars for borders; the title fits in
// width-6 chars.
func (m Model) tabStrip(width int) string {
	labels := []string{"Heute", "Woche", "History", "Frei"}
	short := []string{"H", "W", "Hi", "F"}
	budget := width - 6
	if budget < 1 {
		budget = 1
	}
	for _, opt := range []struct {
		labels []string
		sep    string
	}{
		{labels, "  ·  "},
		{labels, " · "},
		{short, " · "},
	} {
		if out := m.renderTabs(opt.labels, opt.sep); lipgloss.Width(out) <= budget {
			return out
		}
	}
	return m.renderTabs(short, " ")
}

func (m Model) renderTabs(labels []string, sep string) string {
	// A11y-2 — der aktive Tab kriegt zusätzlich zum Bold+Accent-Foreground
	// einen Underline-SGR. Glyph + Color allein liefen Gefahr, in NO_COLOR /
	// Color-Blind-Settings ohne Identifier dazustehen; ein Unterstrich ist
	// der etablierte Identifier (Skill §Tabs „Underline (default)").
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

// tickInterval reports the duration the next tick should fire after.
// Fast (1 s) when the active sub-model reports it via FastTick (e.g.
// Heute during the first minute of a running session); slow (10 s)
// otherwise. Uses the injected Clock so the branch selection stays
// deterministic under a fake clock in tests. Extracted from
// scheduleTick so the duration choice is testable without invoking
// the tea.Tick command (which would block on the real timer).
func (m Model) tickInterval() time.Duration {
	if ft, ok := m.subs[m.current].(fastTicker); ok && ft.FastTick(m.deps.Clock.Now()) {
		return tickFast
	}
	return tickSlow
}

// scheduleTick returns a tea.Cmd that fires after tickInterval().
func (m Model) scheduleTick() tea.Cmd {
	return tea.Tick(m.tickInterval(), func(t time.Time) tea.Msg { return tickMsg(t) })
}

// — interfaces sub-models can opt into —

type filterActiver interface {
	FilterActive() bool
	StateFilter() string
}

type cursorStater interface {
	StateCursor() int
}

// fastTicker lets a sub-model opt into the fast (1 s) tick interval
// while it has a freshly-started session whose seconds-counter is
// visible to the user. Returning false drops to the slow tick.
type fastTicker interface {
	FastTick(now time.Time) bool
}

// textInputActiver lets a sub-model report whether a textinput.Model is
// currently focused — i.e. the user is typing free-form text into a
// field and 'q' should land in the field, not quit the program.
//
// Sub-models that don't implement this default to "no text input
// active" — q in those contexts (Heute idle, Woche, History list,
// menu list / target / land) returns tea.Quit at the worktime root.
type textInputActiver interface {
	TextInputActive() bool
}

// textInputActive aggregates the menu's and the active sub-model's
// text-input state. The worktime root checks this before honouring
// q-as-quit so typing 'q' inside a tag / note / range form / etc.
// edits the field instead of quitting.
func (m Model) textInputActive() bool {
	if m.menu.Active() && m.menu.TextInputActive() {
		return true
	}
	if ti, ok := m.subs[m.current].(textInputActiver); ok {
		return ti.TextInputActive()
	}
	return false
}
