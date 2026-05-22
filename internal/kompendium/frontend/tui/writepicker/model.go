// Package writepicker is the small Bubble Tea picker that backs
// `kompendium write`: a tmux-popup-friendly menu that asks "Daily,
// Project, or Free Note?" and (for Free) collects a slug, leaving the
// outer CLI to invoke the matching Create* use case.
package writepicker

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Choice is the user's pick after the picker exits.
type Choice int

// Defined Choice values.
const (
	// ChoiceCancel signals the user dismissed the picker; the caller does
	// nothing.
	ChoiceCancel Choice = iota
	// ChoiceDaily means the caller should run the CreateDaily use case.
	ChoiceDaily
	// ChoiceProject means CreateProject.
	ChoiceProject
	// ChoiceFree means CreateFree, with Result.Slug as the input.
	ChoiceFree
)

// Result bundles what the picker collected. Slug is set only for ChoiceFree.
type Result struct {
	Choice Choice
	Slug   string
}

// DoneMsg is the tea.Msg the picker emits when the user has finished
// (either submitted a choice or cancelled). The hosting model
// intercepts this to harvest the Result. A standalone CLI host
// converts it into tea.Quit; an embedding host (e.g. kompendium
// browse) returns to its previous mode and acts on the Result without
// tearing down the outer program.
//
// Replaces the previous design where the picker emitted tea.Quit
// directly. That worked for the standalone path, but inside browse
// the picker had to run as a subprocess (`flow kompendium write`) and
// nested tea.Programs through tea.ExecProcess fail at /dev/tty
// negotiation in bubbletea v1.3.x — the picker never appeared and
// the subprocess exited 1.
type DoneMsg struct{ Result Result }

func doneCmd(r Result) tea.Cmd {
	return func() tea.Msg { return DoneMsg{Result: r} }
}

type option struct {
	label  string
	hint   string
	icon   string
	choice Choice
}

// Model is the Bubble Tea state for the picker.
type Model struct {
	options    []option
	cursor     int
	askingSlug bool
	slug       textinput.Model
	result     Result
	quitting   bool
	width      int
	height     int
}

// New returns a picker. When allowProject is false the Project option is
// hidden — that is the case when the caller is not in a git repository.
func New(allowProject bool) Model {
	// Icons aus der Whitelist: Filled / Extra / Empty (vorher ▣ ▦ ▥,
	// die nicht in components/glyphs stehen und in einigen Nerd-Fonts
	// emoji-Breite hatten). Daily = Filled (heute, jetzt), Project =
	// Extra (zusätzliche Note neben dem Daily), Free = Empty (offener
	// Slot ohne Default-Inhalt).
	opts := []option{
		{label: "Daily-Note", hint: "Tagesjournal", icon: glyphs.Filled, choice: ChoiceDaily},
	}
	if allowProject {
		opts = append(opts, option{label: "Projekt-Note", hint: "aktuelles Repo · heute", icon: glyphs.Extra, choice: ChoiceProject})
	}
	opts = append(opts, option{label: "Freie Note", hint: "benannter Slug", icon: glyphs.Empty, choice: ChoiceFree})

	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 64
	ti.Placeholder = "slug"
	tiStyles := ti.Styles()
	tiStyles.Focused.Placeholder = dimStyle
	tiStyles.Blurred.Placeholder = dimStyle
	tiStyles.Cursor.Color = cursorStyle.GetForeground()
	ti.SetStyles(tiStyles)

	return Model{options: opts, slug: ti}
}

// Init satisfies tea.Model. The picker has nothing to schedule on entry.
func (m Model) Init() tea.Cmd { return nil }

// Result reports what the user selected; valid after Update returns
// tea.Quit.
func (m Model) Result() Result { return m.result }

// Update is the Bubble Tea reducer. Returns concrete writepicker.Model
// (not tea.Model) so the picker stays a sub-model — the hosting
// adapter (pickerHost in cli/write.go) implements tea.Model. Under
// bubbletea v2 tea.Model requires View() tea.View; keeping writepicker
// as a plain reducer lets us return View() string for in-process
// composition while the host wraps the rendered content into the
// program-bound tea.View.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		if m.askingSlug {
			return m.handleSlugKey(msg)
		}
		return m.handleMenuKey(msg)
	}
	return m, nil
}

func (m Model) handleMenuKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.quitting = true
		m.result = Result{Choice: ChoiceCancel}
		return m, doneCmd(m.result)
	case "j", "down":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		opt := m.options[m.cursor]
		if opt.choice == ChoiceFree {
			m.askingSlug = true
			m.slug.Focus()
			return m, textinput.Blink
		}
		m.result = Result{Choice: opt.choice}
		m.quitting = true
		return m, doneCmd(m.result)
	}
	return m, nil
}

func (m Model) handleSlugKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.quitting = true
		m.result = Result{Choice: ChoiceCancel}
		return m, doneCmd(m.result)
	case "enter":
		if strings.TrimSpace(m.slug.Value()) == "" {
			return m, nil
		}
		m.result = Result{Choice: ChoiceFree, Slug: strings.TrimSpace(m.slug.Value())}
		m.quitting = true
		return m, doneCmd(m.result)
	}
	var cmd tea.Cmd
	m.slug, cmd = m.slug.Update(msg)
	return m, cmd
}

// View renders the picker.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var card string
	if m.askingSlug {
		card = slugCard(m.slug.View())
	} else {
		card = menuCard(m.options, m.cursor)
	}

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card,
			lipgloss.WithWhitespaceChars("·"),
			lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(pal.BgChip)))
	}
	return card
}

// pal ist die canonical Palette dieses Pickers. Init-Time-Snapshot
// von theme.Default. Composition-Root-Bridge swappt sie über
// SetPalette(p) auf den Live-Wert (= tk.Load() in cli/sidekick.go +
// cmd/flow/main.go); rebuildStyles() weist alle Style-vars neu zu,
// sodass ein @tn_*-tmux-Overlay durchschlägt.
//
// sem ist die Sem()-Sicht — Components lesen den semantischen Alias,
// nicht die rohe Hue (siehe docs/design-system.md).
var (
	pal = theme.Default
	sem = pal.Sem()
)

// SetPalette swappt die Package-Palette und rebuildet alle Styles.
func SetPalette(p theme.Palette) {
	pal = p
	sem = p.Sem()
	rebuildStyles()
}

var (
	frameStyle     lipgloss.Style
	headerStyle    lipgloss.Style
	cursorStyle    lipgloss.Style
	selectedStyle  lipgloss.Style
	optionStyle    lipgloss.Style
	iconStyle      lipgloss.Style
	hintStyle      lipgloss.Style
	dimStyle       lipgloss.Style
	footerStyle    lipgloss.Style
	footerKeyStyle lipgloss.Style
	slugBoxStyle   lipgloss.Style
)

func init() { rebuildStyles() }

func rebuildStyles() {
	frameStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(sem.Accent).
		Padding(1, 3)
	headerStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	cursorStyle = lipgloss.NewStyle().
		Foreground(sem.Accent).
		Bold(true)
	selectedStyle = lipgloss.NewStyle().
		Foreground(pal.Fg).
		Background(pal.BgChip).
		Bold(true).
		Padding(0, 1)
	optionStyle = lipgloss.NewStyle().
		Foreground(pal.Fg).
		Padding(0, 1)
	iconStyle = lipgloss.NewStyle().
		Foreground(sem.Active)
	hintStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	dimStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	footerStyle = lipgloss.NewStyle().
		Foreground(pal.FgMuted)
	footerKeyStyle = lipgloss.NewStyle().
		Foreground(sem.Active).
		Bold(true)
	slugBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(sem.Warning).
		Padding(0, 1)
}

func menuCard(opts []option, cursor int) string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Note anlegen"))
	sb.WriteString("\n\n")
	for i, opt := range opts {
		icon := iconStyle.Render(opt.icon)
		row := icon + "  " + opt.label
		if opt.hint != "" {
			row = row + "  " + hintStyle.Render(opt.hint)
		}
		if i == cursor {
			sb.WriteString(cursorStyle.Render(glyphs.Active + " "))
			sb.WriteString(selectedStyle.Render(row))
		} else {
			sb.WriteString("  ")
			sb.WriteString(optionStyle.Render(row))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\n")
	sb.WriteString(footerLine([]hintEntry{
		{"j/k", "navigieren"},
		{"Enter", "wählen"},
		{"q", "abbrechen"},
	}))
	return frameStyle.Render(sb.String())
}

func slugCard(slugView string) string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Slug für die neue freie Note"))
	sb.WriteString("\n\n")
	if slugView == "" {
		slugView = glyphs.AccentBar
	}
	sb.WriteString(slugBoxStyle.Render(slugView))
	sb.WriteString("\n\n")
	sb.WriteString(footerLine([]hintEntry{
		{"Enter", "bestätigen"},
		{"Esc", "abbrechen"},
	}))
	return frameStyle.Render(sb.String())
}

type hintEntry struct{ key, desc string }

func footerLine(entries []hintEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, footerKeyStyle.Render(e.key)+footerStyle.Render(" "+e.desc))
	}
	return strings.Join(parts, footerStyle.Render(" · "))
}
