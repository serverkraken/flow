package worktime

// noteAttachPicker is the shared widget for attaching a Kompendium note
// to a specific date. Round 5 introduces it as a self-contained surface
// that history-drill can host on an arbitrary past-day date; the heute
// tab still uses its in-line dialog plumbing (see today_dialog_*.go)
// and will be migrated in a follow-up.
//
// Lifecycle:
//
//	p := newNoteAttachPicker(deps, pal)
//	p, cmd := p.Open(date, attachedIDs) // when user presses 'n'
//	// ... per KeyMsg while active:
//	p, cmd, action := p.Update(key)
//	switch action {
//	case noteAttachActionSubmit:
//	    if id := p.SelectedID(); id != "" {
//	        // host fires writer.Add(date, id) via its own tea.Cmd
//	    } else {
//	        p = p.SetError("Note-ID darf nicht leer sein")
//	    }
//	case noteAttachActionCancel:
//	    // host closes its dialog flag
//	}
//	// render: p.View(inner) inside the host's frame
//
// The picker owns only its own input/list/cursor state — the host
// decides when the dialog is "active", when to invoke LinkWriter, and
// how to surface the resulting toast / reload.

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// noteAttachAction is the verdict the picker returns after each KeyMsg.
// noteAttachActionIdle: the host stays in the dialog with the updated
// picker state. Submit/Cancel let the host commit/close.
type noteAttachAction int

const (
	noteAttachActionIdle noteAttachAction = iota
	noteAttachActionSubmit
	noteAttachActionCancel
)

// noteAttachPicker bundles the input, recent-note suggestion list,
// cursor and error slot for an "attach a Kompendium note" dialog.
type noteAttachPicker struct {
	deps Deps
	pal  theme.Palette

	date     time.Time
	input    textinput.Model
	suggs    []NoteSuggestion
	cur      int
	errMsg   string
	attached []string
}

// newNoteAttachPicker constructs a zero-state picker. Call Open() with
// the target date before rendering.
func newNoteAttachPicker(deps Deps, pal theme.Palette) noteAttachPicker {
	return noteAttachPicker{deps: deps, pal: pal}
}

// Open prepares the picker for attaching to date. attachedIDs is the
// list of note IDs already linked to that date (rendered as the
// "bereits angehängt" hint). When deps.NoteLister is non-nil the picker
// pre-fills suggestions from its Recent() call; otherwise it degrades
// to a raw-ID input.
func (p noteAttachPicker) Open(date time.Time, attachedIDs []string) (noteAttachPicker, tea.Cmd) {
	p.date = date
	placeholder := "tippen → suchen, oder Note-ID"
	if p.deps.NoteLister == nil {
		placeholder = "Note-ID (z.B. 2026-05-03 oder daily-2026-05-03)"
	}
	p.input = form.NewTextInput(placeholder, p.pal)
	p.input.SetValue("")
	p.input.Focus()
	p.errMsg = ""
	p.suggs = nil
	p.cur = 0
	p.attached = attachedIDs
	if p.deps.NoteLister != nil {
		p.suggs = p.deps.NoteLister.Recent(noteAttachPickerLimit)
	}
	return p, textinput.Blink
}

// Date returns the date the picker is attaching to. Host typically uses
// it to construct the toast text and to scope the LinkWriter.Add call.
func (p noteAttachPicker) Date() time.Time { return p.date }

// SetError surfaces a validation/IO error in the picker body. Host calls
// this after a submit roundtrip that failed (LinkWriter.Add error).
func (p noteAttachPicker) SetError(msg string) noteAttachPicker {
	p.errMsg = msg
	return p
}

// SelectedID returns the ID the user is about to submit: the picker
// cursor pick when the filtered list has rows, else the raw input
// (trimmed). Empty string means the input was blank — host should
// surface this via SetError rather than firing a writer call.
func (p noteAttachPicker) SelectedID() string {
	if filt := p.filtered(); len(filt) > 0 && p.cur >= 0 && p.cur < len(filt) {
		return filt[p.cur].ID
	}
	return strings.TrimSpace(p.input.Value())
}

// HintLine returns the canonical bottom-of-dialog hint line. Variants
// depend on whether the suggestion list is populated.
func (p noteAttachPicker) HintLine() string {
	if len(p.suggs) > 0 {
		return "↑/↓ → wählen  ·  tippen → filter  ·  Enter → anhängen  ·  Esc → abbrechen"
	}
	return "Enter → anhängen  ·  Esc → abbrechen"
}

// Update routes one KeyMsg into the picker. Returns the updated picker,
// any tea.Cmd produced (textinput.Blink etc.), and an action verdict
// for the host (Idle / Submit / Cancel).
func (p noteAttachPicker) Update(msg tea.KeyPressMsg) (noteAttachPicker, tea.Cmd, noteAttachAction) {
	// Picker-list navigation claims up/down when there are suggestions.
	// Without the guard, up/down would fall to textinput's no-op (single-
	// line input) and the picker cursor wouldn't move.
	if len(p.suggs) > 0 {
		switch msg.String() {
		case "up", "ctrl+p":
			if filt := p.filtered(); len(filt) > 0 {
				p.cur = (p.cur + len(filt) - 1) % len(filt)
			}
			return p, nil, noteAttachActionIdle
		case "down", "ctrl+n":
			if filt := p.filtered(); len(filt) > 0 {
				p.cur = (p.cur + 1) % len(filt)
			}
			return p, nil, noteAttachActionIdle
		}
	}
	switch msg.String() {
	case "esc":
		return p, nil, noteAttachActionCancel
	case "enter":
		return p, nil, noteAttachActionSubmit
	case "tab", "shift+tab":
		// Single-input dialog — nothing to tab to. Swallow so bubbles
		// textinput doesn't insert a literal tab character that would
		// otherwise survive into the typed ID.
		return p, nil, noteAttachActionIdle
	}
	p.errMsg = ""
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	// Filter-Reset des Cursors: bei Tippen kann der vorherige Index
	// out-of-range geraten, wenn das Filter die Liste verkürzt.
	if filt := p.filtered(); p.cur >= len(filt) {
		p.cur = 0
	}
	return p, cmd, noteAttachActionIdle
}

// filtered reduces suggs on the current input substring (case-insensitive,
// matched against ID OR title). Empty query returns all.
func (p noteAttachPicker) filtered() []NoteSuggestion {
	q := strings.ToLower(strings.TrimSpace(p.input.Value()))
	if q == "" || len(p.suggs) == 0 {
		return p.suggs
	}
	out := make([]NoteSuggestion, 0, len(p.suggs))
	for _, s := range p.suggs {
		if strings.Contains(strings.ToLower(s.ID), q) ||
			strings.Contains(strings.ToLower(s.Title), q) {
			out = append(out, s)
		}
	}
	return out
}

// View renders the picker body inside the host's dialog frame. inner is
// the available width inside the frame. The output is the input row,
// the suggestions list, the "bereits angehängt" hint and any error
// message — separated by newlines.
func (p noteAttachPicker) View(inner int) string {
	var rows []string
	rows = append(rows, picker.SectionHeader("note id", inner, p.pal), "  "+p.input.View())
	rows = append(rows, p.renderSuggestions(inner)...)
	if len(p.attached) > 0 {
		rows = append(rows, "", stDim(p.pal,
			"  bereits angehängt:  "+strings.Join(p.attached, "  ·  ")))
	}
	if p.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+p.errMsg, p.pal))
	}
	return strings.Join(rows, "\n")
}

// renderSuggestions paints the suggestion picker rows (or the
// "Keine Treffer im Filter" hint when the filter empties the list).
// Returns nil when there are no suggestions at all (NoteLister
// unwired or empty notebook) — caller's body then just shows input.
func (p noteAttachPicker) renderSuggestions(inner int) []string {
	if len(p.suggs) == 0 {
		return nil
	}
	filt := p.filtered()
	rows := []string{"", picker.SectionHeader("jüngste notizen", inner, p.pal)}
	if len(filt) == 0 {
		rows = append(rows, stDim(p.pal,
			"  Keine Treffer im Filter — Enter nimmt die getippte ID."))
		return rows
	}
	for i, s := range filt {
		label := s.ID
		hint := ""
		if s.Title != "" && s.Title != s.ID {
			hint = s.Title
		}
		rows = append(rows, picker.Row(i == p.cur, label, hint, inner, p.pal))
	}
	return rows
}
