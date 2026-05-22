package worktime

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — messages —

type freiLoadedMsg struct {
	entries []domain.DayOff
	year    int
	err     error
}

type freiActionDoneMsg struct {
	err   error
	toast string
	// year, when non-zero, replaces the displayed year before the
	// follow-up reload. Used by quickAdd-today so the just-added entry
	// is actually visible even if the user was navigating past years.
	year int
}

// — dialog modes —

type freiDialog int

const (
	freiDialogNone freiDialog = iota
	freiDialogAdd
	freiDialogConfirm
)

// frei is the Frei (day-off) sub-model. F4.3 wave E gives it the action
// surface needed for day-off CRUD: add via form (date or range, kind,
// label), quick-add today as Urlaub or Krank, sync gesetzliche Feiertage
// for the displayed year, and delete with confirm. j/k navigates the
// year's entries; h/l/[/] shifts the year window.
type frei struct {
	pal  theme.Palette
	deps Deps

	width int

	entries []domain.DayOff
	cursor  int
	year    int
	loaded  bool
	err     error

	dialog  freiDialog
	form    []textinput.Model
	formCur int
	kindCur int
	errMsg  string

	// confirmModel — kanonisches Delete-Confirm. Skill §Component vocabulary
	// + §Keybind grammar: y/Enter → ja, n/Esc → nein. Vorher hand-rolled mit
	// Enter-as-cancel-Default, was die Konvention der ganzen App invertierte.
	confirmModel *confirm.Model

	// toast — kanonische green-✓ Bestätigung, post-Welle-3 via toast.Model.
	toast *toast.Model
}

func newFrei(p theme.Palette, deps Deps) frei {
	return frei{pal: p, deps: deps}
}

// — capability interfaces —

// FilterActive bubbles up to the root so global tab keys don't intercept
// while the add form or delete confirm has focus.
func (f frei) FilterActive() bool { return f.dialog != freiDialogNone }

// TextInputActive reports whether the Frei add form is the focused
// surface — only the add dialog hosts text inputs. The delete confirm
// (freiDialogConfirm) is a yes/no prompt, not text input, so q from
// there exits the program.
func (f frei) TextInputActive() bool { return f.dialog == freiDialogAdd }

// StateFilter has no meaning here — Frei has no persistent filter expression.
func (f frei) StateFilter() string { return "" }

// StateCursor reports the focused entry index for state persistence.
func (f frei) StateCursor() int { return f.cursor }

// Init kicks off the year load.
func (f frei) Init() tea.Cmd { return f.loadCmd(f.currentYear()) }

func (f frei) currentYear() int {
	if f.year != 0 {
		return f.year
	}
	return f.deps.Clock.Now().Year()
}

func (f frei) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		return f, nil

	case freiLoadedMsg:
		f.loaded = true
		f.err = msg.err
		if msg.err == nil {
			f.entries = msg.entries
			f.year = msg.year
			f.clampCursor()
		}
		return f, nil

	case freiActionDoneMsg:
		f.dialog = freiDialogNone
		f.form = nil
		f.formCur = 0
		f.confirmModel = nil
		f.errMsg = ""
		f.err = msg.err
		if msg.err == nil && msg.year != 0 && msg.year != f.currentYear() {
			f.year = msg.year
			f.cursor = 0
			f.loaded = false
		}
		if msg.err == nil && msg.toast != "" {
			t := toast.NewDefault(msg.toast, f.pal)
			f.toast = &t
			return f, tea.Batch(f.loadCmd(f.currentYear()), t.Init())
		}
		return f, f.loadCmd(f.currentYear())

	case toast.DismissedMsg:
		f.toast = nil
		return f, nil

	case confirm.ResultMsg:
		// Auflösung des Delete-Confirm-Dialogs (siehe today.go für die
		// Begründung des Wechsels auf confirm.Model).
		if f.dialog != freiDialogConfirm {
			return f, nil
		}
		f.dialog = freiDialogNone
		f.confirmModel = nil
		if !msg.Confirmed {
			return f, nil
		}
		if f.cursor < 0 || f.cursor >= len(f.entries) {
			return f, nil
		}
		date := f.entries[f.cursor].Date
		writer := f.deps.DayOffWriter
		return f, func() tea.Msg {
			if err := writer.Remove(date); err != nil {
				return freiActionDoneMsg{err: err}
			}
			return freiActionDoneMsg{toast: "✓ Eintrag entfernt für " + date.Format("2006-01-02")}
		}

	case dayRefreshMsg:
		return f, f.loadCmd(f.currentYear())

	case tea.KeyPressMsg:
		if f.dialog != freiDialogNone {
			return f.handleDialogKey(msg)
		}
		return f.handleNormalKey(msg)
	}
	return f, nil
}

func (f frei) loadCmd(year int) tea.Cmd {
	reader := f.deps.DayOffStore
	return func() tea.Msg {
		from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
		to := time.Date(year, time.December, 31, 0, 0, 0, 0, time.Local)
		return freiLoadedMsg{entries: reader.List(from, to), year: year}
	}
}

func (f *frei) clampCursor() {
	total := len(f.entries)
	if f.cursor >= total {
		f.cursor = total - 1
	}
	if f.cursor < 0 {
		f.cursor = 0
	}
}

// — keymap (no dialog) —

func (f frei) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if total := len(f.entries); total > 0 {
			f.cursor = (f.cursor + 1) % total
		}
	case "k", "up":
		if total := len(f.entries); total > 0 {
			f.cursor = (f.cursor + total - 1) % total
		}
	case "g":
		f.cursor = 0
	case "G":
		if total := len(f.entries); total > 0 {
			f.cursor = total - 1
		}
	case "a":
		return f.openAdd()
	case "A":
		return f, f.quickAddTodayCmd(domain.KindVacation)
	case "K":
		return f, f.quickAddTodayCmd(domain.KindSick)
	case "B":
		return f, f.syncHolidaysCmd()
	case "D":
		// Skill §Keybind grammar: destructive Action = Uppercase. Vorher d/x.
		// `x` als Alias war historischer Ballast — Uppercase D ist der einzige
		// destructive Slot in der ganzen App.
		if f.cursor >= 0 && f.cursor < len(f.entries) {
			f.dialog = freiDialogConfirm
			f.errMsg = ""
			d := f.entries[f.cursor]
			question := "Eintrag löschen?"
			detail := fmt.Sprintf("%s  %s  %s",
				d.Date.Format("2006-01-02"), d.Kind.LabelDe(), d.Label)
			cm := confirm.New(question, detail, f.pal)
			f.confirmModel = &cm
			return f, cm.Init()
		}
	case "h", "left", "[":
		return f.shiftYear(-1)
	case "l", "right", "]":
		return f.shiftYear(+1)
	case "T":
		now := f.deps.Clock.Now()
		f.loaded = false
		f.cursor = 0
		f.year = now.Year()
		return f, f.loadCmd(f.year)
	}
	return f, nil
}

func (f frei) shiftYear(delta int) (tea.Model, tea.Cmd) {
	f.loaded = false
	f.cursor = 0
	f.year = f.currentYear() + delta
	return f, f.loadCmd(f.year)
}

// — actions —

func (f frei) quickAddTodayCmd(kind domain.Kind) tea.Cmd {
	writer := f.deps.DayOffWriter
	now := f.deps.Clock.Now()
	return func() tea.Msg {
		if err := writer.Add(now, kind, ""); err != nil {
			return freiActionDoneMsg{err: err}
		}
		// Quick-add always lands on today, but the list shows the
		// currently selected year. Hopping back to today's year so
		// the new entry is visible — otherwise the toast says "✓
		// eingetragen" while the empty 2024 grid stares back.
		return freiActionDoneMsg{
			year: now.Year(),
			toast: fmt.Sprintf("✓ %s eingetragen für %s",
				kind.LabelDe(), now.Format("2006-01-02")),
		}
	}
}

func (f frei) syncHolidaysCmd() tea.Cmd {
	writer := f.deps.DayOffWriter
	year := f.currentYear()
	land := landOrDefault(f.deps.Land)
	return func() tea.Msg {
		added, _, err := writer.SyncGermanHolidays(year, land, time.Local)
		if err != nil {
			return freiActionDoneMsg{err: err}
		}
		return freiActionDoneMsg{toast: fmt.Sprintf("✓ %d Feiertag(e) für %s/%d", added, land, year)}
	}
}

// — add dialog —

func (f frei) openAdd() (tea.Model, tea.Cmd) {
	now := f.deps.Clock.Now()
	date := form.NewTextInput("YYYY-MM-DD oder YYYY-MM-DD..YYYY-MM-DD", f.pal)
	date.SetValue(now.Format("2006-01-02"))
	date.CursorEnd()
	label := form.NewTextInput("z.B. Brückentag", f.pal)
	date.Focus()
	f.form = []textinput.Model{date, label}
	f.formCur = 0
	f.kindCur = 1 // default: Urlaub (most common manual add)
	f.dialog = freiDialogAdd
	f.errMsg = ""
	return f, textinput.Blink
}

// — dialog dispatch —

func (f frei) handleDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch f.dialog {
	case freiDialogConfirm:
		// confirm.Model konsumiert y/Enter (ja) und n/Esc (nein); Result
		// wird im Outer-Update als confirm.ResultMsg gehandhabt.
		if f.confirmModel == nil {
			f.dialog = freiDialogNone
			return f, nil
		}
		updated, cmd := f.confirmModel.Update(msg)
		f.confirmModel = &updated
		return f, cmd
	case freiDialogAdd:
		return f.handleAddFormKey(msg)
	}
	return f, nil
}

// kindIdx is the formCur value that addresses the kind picker (which
// is a "virtual" form field rendered after the text inputs). Centralised
// so the picker stays reachable when text fields are added — the older
// `len(f.form)` arithmetic was correct only because there happened to
// be exactly two text fields.
func (f frei) kindIdx() int { return len(f.form) }

func (f frei) handleAddFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	last := f.kindIdx()
	switch msg.String() {
	case "esc":
		f.dialog = freiDialogNone
		f.form = nil
		f.errMsg = ""
		return f, nil
	case "tab", "down":
		next := f.formCur + 1
		if next > last {
			next = 0
		}
		f.focusForm(next)
		return f, textinput.Blink
	case "shift+tab", "up":
		next := f.formCur - 1
		if next < 0 {
			next = last
		}
		f.focusForm(next)
		return f, textinput.Blink
	case "enter":
		if f.formCur < last {
			f.focusForm(f.formCur + 1)
			return f, textinput.Blink
		}
		return f.submitAdd()
	}
	if f.formCur == last {
		return f.handleKindCycle(msg)
	}
	if f.formCur >= 0 && f.formCur < last {
		var cmd tea.Cmd
		f.errMsg = ""
		f.form[f.formCur], cmd = f.form[f.formCur].Update(msg)
		return f, cmd
	}
	return f, nil
}

func (f frei) handleKindCycle(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		if f.kindCur > 0 {
			f.kindCur--
		} else {
			f.kindCur = len(domain.AllKinds) - 1
		}
	case "l", "right":
		f.kindCur = (f.kindCur + 1) % len(domain.AllKinds)
	}
	return f, nil
}

func (f *frei) focusForm(i int) {
	for j := range f.form {
		if j == i {
			f.form[j].Focus()
		} else {
			f.form[j].Blur()
		}
	}
	f.formCur = i
}

func (f frei) submitAdd() (tea.Model, tea.Cmd) {
	dateExpr := strings.TrimSpace(f.form[0].Value())
	label := strings.TrimSpace(f.form[1].Value())
	from, to, isRange, err := domain.ParseDateOrRange(dateExpr, time.Local)
	if err != nil {
		f.errMsg = err.Error()
		return f, nil
	}
	kind := domain.AllKinds[f.kindCur%len(domain.AllKinds)]
	writer := f.deps.DayOffWriter
	if isRange {
		return f, func() tea.Msg {
			n, err := writer.AddRange(from, to, kind, label)
			if err != nil {
				return freiActionDoneMsg{err: err}
			}
			return freiActionDoneMsg{toast: fmt.Sprintf("✓ %d Tag(e) als %s eingetragen",
				n, kind.LabelDe())}
		}
	}
	return f, func() tea.Msg {
		if err := writer.Add(from, kind, label); err != nil {
			return freiActionDoneMsg{err: err}
		}
		return freiActionDoneMsg{toast: fmt.Sprintf("✓ %s eingetragen für %s",
			kind.LabelDe(), from.Format("2006-01-02"))}
	}
}

// — render —

func (f frei) View() string {
	if f.width == 0 {
		return ""
	}
	if f.dialog != freiDialogNone {
		return f.renderDialog()
	}
	if !f.loaded {
		return stDim(f.pal, "  Frei lädt …")
	}
	if f.err != nil {
		return stErr(f.pal, f.err.Error())
	}
	return f.renderBody()
}

func (f frei) renderBody() string {
	inner := f.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{f.renderHeader(), ""}
	rows = append(rows, f.renderEntries(inner)...)
	// Toast-Slot via SlotRows — kollabiert, wenn kein Toast aktiv ist,
	// statt drei Leerzeilen unter der Liste zu hinterlassen.
	rows = append(rows, toast.SlotRows(f.toast, "  ")...)
	rows = append(rows, "", renderFooterHints(f.pal, f.footerHints(), inner))
	return strings.Join(rows, "\n")
}

func (f frei) renderHeader() string {
	year := f.currentYear()
	now := f.deps.Clock.Now()
	left := theme.Heading(fmt.Sprintf("Frei %d", year), f.pal)
	if year == now.Year() {
		left += "   " + stDim(f.pal, "aktuelles Jahr")
	}
	return "  " + left
}

func (f frei) renderEntries(inner int) []string {
	if len(f.entries) == 0 {
		// Empty-state: B-Sync (gesetzliche Feiertage) ist die häufigste
		// First-Action und steht deshalb visuell vorne — Hervorhebung mit
		// Highlight statt Dim, plus Bundesland-Hint aus WORKTIME_LAND damit
		// der User sofort sieht *was* gesynct wird. a/A/K liegen darunter
		// als sekundäre Hints im 4-Hint-Slot.
		land := landOrDefault(f.deps.Land)
		bSyncHint := theme.Highlight("B", f.pal) + theme.Dim(
			fmt.Sprintf("  → gesetzliche Feiertage für %s importieren", land), f.pal,
		)
		hint := []string{"a → anlegen", "A → heute=Urlaub", "K → heute=krank"}
		return []string{
			stDim(f.pal, "  Noch keine Daten in diesem Jahr."),
			"",
			"  " + bSyncHint,
			"",
			renderFooterHints(f.pal, hint, inner),
		}
	}
	rows := []string{"  " + f.renderKindSummary(), ""}
	rows = append(rows, picker.SectionHeader(fmt.Sprintf("einträge (%d)", len(f.entries)), inner, f.pal))
	for i, d := range f.entries {
		rows = append(rows, f.renderEntryRow(i, d, inner))
	}
	return rows
}

func (f frei) renderKindSummary() string {
	byKind := map[domain.Kind]int{}
	for _, d := range f.entries {
		byKind[d.Kind]++
	}
	parts := make([]string, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		if c := byKind[k]; c > 0 {
			labelStyle := lipgloss.NewStyle().Foreground(theme.KindColor(f.pal, k))
			parts = append(parts,
				labelStyle.Render(k.LabelDe())+" "+stDim(f.pal, fmt.Sprintf("%d", c)))
		}
	}
	return strings.Join(parts, stDim(f.pal, "  ·  "))
}

func (f frei) renderEntryRow(idx int, d domain.DayOff, inner int) string {
	// Spec 2026-05-13-filled-dayoff-dots-supersede §Frei-view: alle Kind-
	// gebundenen Texte (kind label + user label) tragen dieselbe Kind-Farbe.
	// Datum bleibt FgMuted als kontextuelles Präfix; eine zusätzliche dim-
	// Farbe verletzt nicht "one accent per row", weil Datum kein Akzent ist.
	//
	// kindBase teilen wir zwischen kindCell und labelCell, damit das §2.6
	// NewStyle-Budget für dayoffs.go nicht aufgepumpt wird.
	date := domain.WeekdayShortDe(d.Date.Weekday()) + " " + d.Date.Format("02.01.")
	kindBase := lipgloss.NewStyle().Foreground(theme.KindColor(f.pal, d.Kind))
	dateCell := lipgloss.NewStyle().Width(10).Foreground(f.pal.FgMuted).Render(date)
	kindCell := kindBase.Width(10).Render(d.Kind.LabelDe())
	labelCell := kindBase.Render(d.Label)
	row := dateCell + "  " + kindCell + "  " + labelCell
	return picker.Row(idx == f.cursor, row, "", inner, f.pal)
}

// footerHints — Skill §Hint format: max 4. Top-4 nach Frequenz:
// navigieren, Eintrag anlegen, löschen, Aktions-Menü. A/K/B/T und das
// Jahr-Blättern (h/l/[/]) leben im `?`-Overlay, das Aktions-Menü
// erlaubt sowieso einen Land-spezifischen Sync außerhalb des `B`-
// Default-Pfads.
func (f frei) footerHints() []string {
	return []string{
		"j/k → bewegen",
		"a → anlegen",
		"D → löschen",
		": → aktionen",
	}
}

// — render dialog —

func (f frei) renderDialog() string {
	inner := f.width - 4
	if inner <= 0 {
		inner = 80
	}
	switch f.dialog {
	case freiDialogAdd:
		return f.renderAddDialog(inner)
	case freiDialogConfirm:
		return f.renderConfirmDialog(inner)
	}
	return ""
}

func (f frei) renderAddDialog(inner int) string {
	rows := []string{
		// Skill §Component vocabulary: Dialog-Title in Purple-Bold (Identity)
		// statt Accent — konsistent mit titlebox/help-Konvention.
		theme.Highlight("  Tag(e) frei eintragen", f.pal),
		"",
	}
	rows = append(rows, f.renderAddFields(inner)...)
	rows = append(rows, f.renderKindPicker(inner))
	if f.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+f.errMsg, f.pal))
	}
	// Form-internal: Tab navigiert Felder (im Input feuert j/k auf die
	// Buchstaben, deshalb Tab als Primary). h/l wechseln Kategorien —
	// der Section-Header des Pickers ("kategorie (h/l zum Wechseln)")
	// sagt schon, wo h/l greift, das Parenthetical ist deshalb raus.
	rows = append(rows, "", renderFooterHints(f.pal, []string{
		"Tab → Feld",
		"h/l → Kategorie",
		"Enter → weiter / speichern",
		"Esc → abbrechen",
	}, inner))
	return strings.Join(rows, "\n")
}

func (f frei) renderAddFields(inner int) []string {
	labels := []string{"datum", "label"}
	rows := make([]string, 0, len(f.form)*2)
	for i, ti := range f.form {
		rows = append(rows, renderFormField(labels[i], ti, i == f.formCur, inner, f.pal)...)
	}
	return rows
}

func (f frei) renderKindPicker(inner int) string {
	header := picker.SectionHeader("kategorie  (h/l zum Wechseln)", inner, f.pal)
	chips := make([]string, 0, len(domain.AllKinds))
	kindFocused := f.formCur == f.kindIdx()
	for i, k := range domain.AllKinds {
		// Spec 2026-05-13-filled-dayoff-dots-supersede §Frei-view: Glyph
		// und Label des unselektierten Chips teilen die Kind-Farbe — die
		// Kind-Identität liest sich konsistent über den ganzen Tab. Der
		// selektierte Chip behält die Accent-Selektions-Behandlung;
		// §Color semantics "one accent per row" bleibt gewahrt, weil die
		// Kind-Farbe in unselektierten Chips kein Akzent ist sondern eine
		// Identitätsfarbe.
		color := theme.KindColor(f.pal, k)
		glyphStyle := lipgloss.NewStyle().Foreground(color)
		labelStyle := lipgloss.NewStyle().Foreground(color)
		if i == f.kindCur {
			if kindFocused {
				labelStyle = lipgloss.NewStyle().Foreground(f.pal.Bg).Background(f.pal.Sem().Accent).Bold(true).Underline(true)
			} else {
				labelStyle = lipgloss.NewStyle().Foreground(f.pal.Sem().Accent).Bold(true).Underline(true)
			}
			glyphStyle = labelStyle
		}
		chips = append(chips,
			glyphStyle.Render(" "+glyphs.Filled+" ")+labelStyle.Render(k.LabelDe()+" "))
	}
	return header + "\n  " + strings.Join(chips, "  ")
}

func (f frei) renderConfirmDialog(_ int) string {
	rows := []string{
		// Title konsistent zu renderAddDialog: Purple-Bold.
		theme.Highlight("  Eintrag löschen", f.pal),
		"",
	}
	if f.confirmModel != nil {
		rows = append(rows, "  "+f.confirmModel.View())
	}
	return strings.Join(rows, "\n")
}

// — pure helpers (private to package) —
