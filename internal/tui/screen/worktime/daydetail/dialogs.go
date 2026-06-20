package daydetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

// nachbuchenFocus enumerates the focus order in the Nachbuchen dialog.
type nachbuchenFocus int

const (
	focusProj nachbuchenFocus = iota
	focusVon
	focusBis
	focusTag
	focusNote
)

// nachbuchenState holds the model for the Nachbuchen (Add) dialog.
type nachbuchenState struct {
	proj   fuzzylist.Model
	projID *string // resolved after project is selected
	von    textinput.Model
	bis    textinput.Model
	tag    textinput.Model
	note   textinput.Model
	focus  nachbuchenFocus
}

// projectItems converts a domain project slice to fuzzylist items.
func projectItems(ps []domain.Project) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(ps))
	for _, p := range ps {
		out = append(out, fuzzylist.Item{ID: p.ID, Label: p.Name})
	}
	return out
}

// openNachbuchen constructs the initial dialog state with the given project list.
func openNachbuchen(pal theme.Palette, projects []domain.Project) *nachbuchenState {
	proj := fuzzylist.New(projectItems(projects), pal).WithCreateHint("neu: %s")
	von := form.NewTextInput("HH:MM", pal)
	bis := form.NewTextInput("HH:MM oder +1h30m", pal)
	tag := form.NewTextInput("z.B. deep, meeting", pal)
	note := form.NewTextInput("kurzer Text", pal)
	return &nachbuchenState{
		proj:  proj,
		von:   von,
		bis:   bis,
		tag:   tag,
		note:  note,
		focus: focusProj, // start on project picker
	}
}

// nachbuchenProjectMsg is sent when an inline project create resolves.
type nachbuchenProjectMsg struct {
	id  string
	err error
}

// nachbuchenDoneMsg is sent when AddSession completes (or errors).
type nachbuchenDoneMsg struct {
	err error
}

// nachbuchenLoadProjectsMsg carries the project list for opening the dialog.
type nachbuchenLoadProjectsMsg struct {
	projects []domain.Project
	err      error
}

// CapturesInput implements shell.InputCapturer — while the Nachbuchen dialog is
// open the shell must forward all keys to this route directly.
func (r *Route) CapturesInput() bool { return r.nachb != nil }

// handleNachbuchenKey processes a key press while the Nachbuchen dialog is open.
func (r *Route) handleNachbuchenKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	// Esc always cancels.
	if k.Code == tea.KeyEsc {
		r.nachb = nil
		return r, nil
	}

	nb := r.nachb

	// When focus is on the project picker, forward keys to fuzzylist.
	if nb.focus == focusProj {
		switch k.Code {
		case tea.KeyEnter:
			it, isCreate, ok := nb.proj.Selection()
			if !ok {
				return r, nil
			}
			if isCreate {
				name := strings.TrimSpace(nb.proj.Query())
				api := r.api
				return r, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					p, err := api.CreateProject(ctx, name)
					if err != nil {
						return nachbuchenProjectMsg{err: err}
					}
					return nachbuchenProjectMsg{id: p.ID}
				}
			}
			// Existing project selected — advance to Von.
			id := it.ID
			nb.projID = &id
			nb.focus = focusVon
			_ = nb.von.Focus()
			r.nachb = nb
			return r, nil
		default:
			nb.proj = nb.proj.Update(k)
			r.nachb = nb
			return r, nil
		}
	}

	// Text-field focus: Tab / Shift+Tab / Up / Down cycle fields.
	switch {
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		r.nachbFocus(-1)
		return r, nil
	case k.Code == tea.KeyTab || k.Code == tea.KeyDown:
		r.nachbFocus(+1)
		return r, nil
	case k.Code == tea.KeyUp:
		r.nachbFocus(-1)
		return r, nil
	case k.Code == tea.KeyEnter:
		if nb.focus == focusNote {
			return r.submitNachbuchen()
		}
		r.nachbFocus(+1)
		return r, nil
	}

	// Delegate keystroke to the focused text input.
	var cmd tea.Cmd
	nb = r.nachb
	switch nb.focus {
	case focusVon:
		nb.von, cmd = nb.von.Update(k)
	case focusBis:
		nb.bis, cmd = nb.bis.Update(k)
	case focusTag:
		nb.tag, cmd = nb.tag.Update(k)
	case focusNote:
		nb.note, cmd = nb.note.Update(k)
	}
	r.nachb = nb
	return r, cmd
}

// nachbFocus moves focus by delta (±1) across [focusProj..focusNote], clamping at
// the edges instead of wrapping.
func (r *Route) nachbFocus(d int) {
	nb := r.nachb
	r.blurNachbField(nb)
	cur := int(nb.focus) + d
	if cur < int(focusProj) {
		cur = int(focusProj)
	}
	if cur > int(focusNote) {
		cur = int(focusNote)
	}
	nb.focus = nachbuchenFocus(cur)
	r.nachb = nb
	r.focusNachbField()
}

func (r *Route) blurNachbField(nb *nachbuchenState) {
	switch nb.focus {
	case focusVon:
		nb.von.Blur()
	case focusBis:
		nb.bis.Blur()
	case focusTag:
		nb.tag.Blur()
	case focusNote:
		nb.note.Blur()
	}
}

func (r *Route) focusNachbField() {
	nb := r.nachb
	switch nb.focus {
	case focusVon:
		_ = nb.von.Focus()
	case focusBis:
		_ = nb.bis.Focus()
	case focusTag:
		_ = nb.tag.Focus()
	case focusNote:
		_ = nb.note.Focus()
	}
}

// submitNachbuchen validates fields and calls api.AddSession.
func (r *Route) submitNachbuchen() (shell.Route, tea.Cmd) {
	nb := r.nachb
	vonStr := strings.TrimSpace(nb.von.Value())
	bisStr := strings.TrimSpace(nb.bis.Value())
	tag := strings.TrimSpace(nb.tag.Value())
	note := strings.TrimSpace(nb.note.Value())

	vonD, err := wtfmt.ParseHM(vonStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r, r.toast.Init()
	}
	startTime := r.day.Add(vonD)
	stopTime, err := wtfmt.ParseStop(wtfmt.NormalizeDurationArg(bisStr), startTime, time.Now())
	if err != nil {
		r.toast = toast.NewDanger("Stop ungültig", r.pal)
		return r, r.toast.Init()
	}

	projID := nb.projID
	api := r.api
	r.nachb = nil // close dialog; on error a toast is shown (dialog stays closed)
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, addErr := api.AddSession(ctx, projID, startTime, stopTime, tag, note)
		if addErr != nil {
			return nachbuchenDoneMsg{err: addErr}
		}
		return nachbuchenDoneMsg{}
	}
}

// renderNachbuchen renders the Nachbuchen overlay into the given frame.
func (r *Route) renderNachbuchen(f shell.Frame) string {
	nb := r.nachb
	if nb == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n  Nachbuchen  ")
	b.WriteString(theme.Dim("tab wechselt · enter speichert · esc bricht ab", f.Pal))
	b.WriteString("\n\n")

	if nb.focus == focusProj {
		b.WriteString("  Projekt wählen (tippen → filtern  ·  ↑/↓ → wählen  ·  enter → weiter):\n\n")
		b.WriteString(nb.proj.View(f.Width - 4))
		return b.String()
	}

	// Text-field phase.
	if nb.projID != nil {
		fmt.Fprintf(&b, "  Projekt: %s\n\n", *nb.projID)
	} else {
		b.WriteString("  Projekt: (kein)\n\n")
	}
	labels := []string{"Von  ", "Bis  ", "Tag  ", "Notiz"}
	fields := []textinput.Model{nb.von, nb.bis, nb.tag, nb.note}
	for i, ti := range fields {
		fmt.Fprintf(&b, "  %s %s\n", labels[i], ti.View())
	}
	return b.String()
}

// nachbuchenHints returns the key-hint strip for the Nachbuchen dialog.
func nachbuchenHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "enter", Desc: "speichern"},
		{Key: "esc", Desc: "abbrechen"},
	}
}
