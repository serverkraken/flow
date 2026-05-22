// Range-Form für das Worktime-Aktions-Menü. 1-Feld-Eingabe mit Default,
// Live-Echo via bubbles.textinput und Submit-Validierung gegen
// domain.ParseRange. Wird von Export/Stats konsumiert; Brief skippt
// diese Form (fixe Range über die Action-Variante).
//
// Skill §Component vocabulary: form.NewTextInput für die Input-Zelle,
// picker.SectionHeader als Frame, dim Beispiel-Strip + roter errMsg
// bei Validation-Fehler. Esc cancelt; Enter submitted.

package worktime

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// rangeForm is the modal-state of the range sub-picker. Carries the
// textinput, the original default expression (kept for a potential
// `T`-reset later) and the most recent validation error.
type rangeForm struct {
	input   textinput.Model
	initial string
	errMsg  string
	parent  string // pending-action label, used by view() as the title
}

// newRangeForm primes the form with defaultExpr pre-filled. The
// caller passes the action's default range (e.g. "month" for stats,
// "month" for export — both follow the `flow worktime …` CLI default).
// parentLabel is rendered as the modal's purple-bold title so the user
// always sees what they're picking a range for.
func newRangeForm(p theme.Palette, defaultExpr, parentLabel string) rangeForm {
	in := form.NewTextInput("z.B. today, week, month, 2026, 2026-04-01..2026-04-30", p)
	in.SetValue(defaultExpr)
	in.CursorEnd()
	in.Focus()
	return rangeForm{input: in, initial: defaultExpr, parent: parentLabel}
}

// rangeEvent is the result of one keystroke through the form. The
// menu inspects it: canceled rolls back to the action list; submitted
// forwards expr to the next sub-mode (target picker); otherwise the
// menu just collects any cursor-blink cmd and stays in range mode.
type rangeEvent struct {
	submitted bool
	canceled  bool
	expr      string
}

// handleKey routes a key into the form. Esc cancels; Enter validates
// the expression against domain.ParseRange (clock-anchored) and either
// surfaces the parser error in errMsg or returns submitted=true with
// the trimmed expression. Other keys go through the underlying
// textinput and surface the bubble's cursor-blink cmd.
//
// Empty expression is accepted — downstream callers (export, stats)
// interpret it as "all time" by virtue of domain.ParseRange returning
// the zero-Range.
func (r rangeForm) handleKey(msg tea.KeyPressMsg, now time.Time) (rangeForm, tea.Cmd, rangeEvent) {
	switch msg.String() {
	case "esc":
		return r, nil, rangeEvent{canceled: true}
	case "enter":
		expr := strings.TrimSpace(r.input.Value())
		if expr != "" {
			if _, err := domain.ParseRange(now, expr); err != nil {
				r.errMsg = err.Error()
				return r, nil, rangeEvent{}
			}
		}
		r.errMsg = ""
		return r, nil, rangeEvent{submitted: true, expr: expr}
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	r.errMsg = ""
	return r, cmd, rangeEvent{}
}

// view renders the range form: parent-action title, range input,
// example strip, optional validation error, footer hints. Mirrors
// menu_target.view's shape so both sub-modes feel identical.
func (r rangeForm) view(pal theme.Palette, inner int) string {
	rows := []string{
		theme.Highlight("  Aktion · "+r.parent, pal),
		"",
		picker.SectionHeader("range", inner, pal),
		"  " + r.input.View(),
		"",
		theme.Dim("  Beispiele:  today  ·  week  ·  month  ·  2026  ·  2026-04  ·  2026-04-01..2026-04-30", pal),
	}
	if r.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+r.errMsg, pal))
	}
	rows = append(rows, "", renderFooterHints(pal, []string{
		"enter → weiter",
		"leer → alles",
		"esc → zurück",
	}, inner))
	return strings.Join(rows, "\n")
}
