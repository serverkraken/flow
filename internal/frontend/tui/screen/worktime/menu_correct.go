// Korrektur-Flow für die laufende Heute-Session. 1-Feld-Form (HH:MM),
// validiert via domain.ParseHM, schreibt mit SessionWriter.CorrectStart.
// Predicate gated auf Heute + IsRunning — siehe menu_actions.go; das
// Form öffnet sich also nur, wenn es Sinn macht.

package worktime

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

// correctForm is the modal-state of the start-time correction sub-mode.
// Holds the textinput, the parent action's label (for the title), and
// the most recent validation error.
type correctForm struct {
	input  textinput.Model
	parent string
	errMsg string
}

// newCorrectForm primes the form with the current running session's
// start time as the default — the user usually wants to nudge it by a
// few minutes, and a pre-filled value beats reformatting from scratch.
func newCorrectForm(p theme.Palette, parentLabel, defaultVal string) correctForm {
	in := form.NewTextInput("HH:MM", p)
	in.SetValue(defaultVal)
	in.CursorEnd()
	in.Focus()
	return correctForm{input: in, parent: parentLabel}
}

// correctEvent is the submission outcome of one keystroke through the
// form: canceled rolls back to the action list; submitted carries the
// parsed time anchored on `today` (so HH:MM means today's HH:MM).
type correctEvent struct {
	submitted bool
	canceled  bool
	parsed    time.Time
}

// handleKey routes a key into the form. Esc cancels; Enter validates
// against domain.ParseHM, anchors the duration on today's date and
// returns submitted=true with the resolved time.Time. Other keys go
// through the underlying textinput.
func (c correctForm) handleKey(msg tea.KeyMsg, today time.Time) (correctForm, tea.Cmd, correctEvent) {
	switch msg.Type {
	case tea.KeyEsc:
		return c, nil, correctEvent{canceled: true}
	case tea.KeyEnter:
		v := strings.TrimSpace(c.input.Value())
		if v == "" {
			c.errMsg = "Zeit darf nicht leer sein"
			return c, nil, correctEvent{}
		}
		hm, err := domain.ParseHM(v)
		if err != nil {
			c.errMsg = err.Error()
			return c, nil, correctEvent{}
		}
		base := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
		c.errMsg = ""
		return c, nil, correctEvent{submitted: true, parsed: base.Add(hm)}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	c.errMsg = ""
	return c, cmd, correctEvent{}
}

// view renders the correction form: parent-action title, the HH:MM
// input, optional validation error, footer hints.
func (c correctForm) view(p theme.Palette, inner int) string {
	rows := []string{
		theme.Highlight("  Aktion · "+c.parent, p),
		"",
		picker.SectionHeader("startzeit", inner, p),
		"  " + c.input.View(),
	}
	if c.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+c.errMsg, p))
	}
	rows = append(rows, "", renderFooterHints(p, []string{
		"enter → speichern",
		"esc → zurück",
	}, inner))
	return strings.Join(rows, "\n")
}

// correctCmd dispatches SessionWriter.CorrectStart with the parsed
// time. Returns a tea.Cmd that yields the menu-shaped done message.
// Errors land in errMsg via applyActionDone; success surfaces a green
// toast carrying the new HH:MM.
func correctCmd(deps Deps, ts time.Time) tea.Cmd {
	return func() tea.Msg {
		if deps.SessionWriter == nil {
			return menuActionDoneMsg{err: fmt.Errorf("session writer nicht verdrahtet")}
		}
		if err := deps.SessionWriter.CorrectStart(ts); err != nil {
			return menuActionDoneMsg{err: err}
		}
		return menuActionDoneMsg{toast: fmt.Sprintf("✓ Startzeit auf %s korrigiert", ts.Format("15:04"))}
	}
}

// correctDefaultFor returns the prefilled HH:MM the form should open
// with — the active session's Start time when one is running, or the
// current wallclock time as a fallback. Callers gate this on the
// Heute-+IsRunning predicate; the fallback only matters in pathological
// races (predicate passed but the session ended between check and open).
func correctDefaultFor(deps Deps) string {
	if deps.Reader != nil {
		if day, err := deps.Reader.Today(); err == nil && day.Active != nil {
			return day.Active.Format("15:04")
		}
	}
	if deps.Clock != nil {
		return deps.Clock.Now().Format("15:04")
	}
	return ""
}
