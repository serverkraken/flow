package daydetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/timefmt"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
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
	proj     fuzzylist.Model
	projID   *string // resolved after project is selected
	projName string  // display name of the selected project (empty until picked)
	von      textinput.Model
	bis      textinput.Model
	tag      textinput.Model
	note     textinput.Model
	focus    nachbuchenFocus
}

// engagementItems filters a node slice to engagements and converts them to
// fuzzylist items. Non-engagement nodes (repos, vorhaben, branches) are omitted.
func engagementItems(nodes []domain.Node) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(nodes))
	for _, n := range nodes {
		if n.Kind == domain.KindEngagement {
			out = append(out, fuzzylist.Item{ID: n.ID, Label: n.Name})
		}
	}
	return out
}

// openNachbuchen constructs the initial dialog state with the given project list.
func openNachbuchen(pal theme.Palette, projects []domain.Node) *nachbuchenState {
	proj := fuzzylist.New(engagementItems(projects), pal).WithCreateHint("neu: %s")
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
	id   string
	name string
	err  error
}

// nachbuchenDoneMsg is sent when AddSession completes (or errors).
type nachbuchenDoneMsg struct {
	err error
}

// nachbuchenLoadProjectsMsg carries the project list for opening the dialog.
type nachbuchenLoadProjectsMsg struct {
	projects []domain.Node
	err      error
}

// CapturesInput implements shell.InputCapturer — while any modal (Nachbuchen,
// edit, or delete confirm) is open the shell must forward all keys to this route
// directly instead of treating them as navigation.
func (r *Route) CapturesInput() bool { return r.nachb != nil || r.edit != nil || r.del != nil }

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
					n, err := api.CreateNode(ctx, apiclient.CreateNodeFields{Name: name, Kind: string(domain.KindEngagement)})
					if err != nil {
						return nachbuchenProjectMsg{err: err}
					}
					return nachbuchenProjectMsg{id: n.ID, name: n.Name}
				}
			}
			// Existing project selected — advance to Von.
			id := it.ID
			nb.projID = &id
			nb.projName = it.Label
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
	tags := strings.Fields(nb.tag.Value())
	note := strings.TrimSpace(nb.note.Value())

	vonD, err := timefmt.ParseHM(vonStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r, r.toast.Init()
	}
	startTime := r.day.Add(vonD)
	stopTime, err := timefmt.ParseStop(timefmt.NormalizeDurationArg(bisStr), startTime, time.Now())
	if err != nil {
		r.toast = toast.NewDanger("Stop ungültig", r.pal)
		return r, r.toast.Init()
	}

	projID := nb.projID
	api := r.api
	// Do NOT close the dialog here. It is cleared only in the
	// nachbuchenDoneMsg success branch, so an error (e.g. overlap/409) keeps
	// the dialog open and populated and the user never loses typed input.
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, addErr := api.AddSession(ctx, projID, startTime, stopTime, tags, note)
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
		b.WriteString("  Engagement wählen (tippen → filtern  ·  ↑/↓ → wählen  ·  enter → weiter):\n\n")
		b.WriteString(nb.proj.View(f.Width - 4))
		return b.String()
	}

	// Text-field phase.
	if nb.projName != "" {
		fmt.Fprintf(&b, "  Engagement: %s\n\n", nb.projName)
	} else if nb.projID != nil {
		fmt.Fprintf(&b, "  Engagement: %s\n\n", *nb.projID)
	} else {
		b.WriteString("  Engagement: (kein)\n\n")
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
		{Key: "tab/↑↓", Desc: "Feld"},
		{Key: "enter", Desc: "speichern"},
		{Key: "esc", Desc: "abbrechen"},
	}
}

// ---- Edit dialog -----------------------------------------------------------

// editFocus enumerates the focus order in the edit dialog.
type editFocus int

const (
	editVon editFocus = iota
	editBis
	editTag
	editNote
)

// editState holds the model for the edit (bearbeiten) dialog. It mirrors Today's
// editState but lives here to keep daydetail free of the worktime package.
type editState struct {
	id    string
	date  time.Time // start-of-day of the edited session's date
	von   textinput.Model
	bis   textinput.Model
	tag   textinput.Model
	note  textinput.Model
	focus editFocus
}

// editDoneMsg is sent when EditSession completes (or errors).
type editDoneMsg struct {
	err error
}

// openEdit builds the edit dialog, prefilling Von/Bis (HH:MM) and Tag/Notiz from
// the selected row. The session date is normalised to midnight so submit can add
// the parsed HH:MM offset back onto it.
func (r *Route) openEdit(row dayRow) tea.Cmd {
	von := form.NewTextInput("HH:MM", r.pal)
	von.SetValue(row.Start.Format("15:04"))
	bis := form.NewTextInput("HH:MM oder +1h30m", r.pal)
	if !row.Running {
		bis.SetValue(row.Stop.Format("15:04"))
	}
	tag := form.NewTextInput("z.B. deep meeting (Leerzeichen trennt)", r.pal)
	tag.SetValue(strings.Join(row.Tags, " "))
	note := form.NewTextInput("kurzer Text", r.pal)
	note.SetValue(row.Note)

	y, m, d := row.Start.Date()
	date := time.Date(y, m, d, 0, 0, 0, 0, row.Start.Location())
	cmd := von.Focus()
	r.edit = &editState{
		id:    row.ID,
		date:  date,
		von:   von,
		bis:   bis,
		tag:   tag,
		note:  note,
		focus: editVon,
	}
	return cmd
}

// handleEditKey processes a key press while the edit dialog is open.
func (r *Route) handleEditKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.edit = nil
		return r, nil
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		r.editFocusBy(-1)
		return r, nil
	case k.Code == tea.KeyTab || k.Code == tea.KeyDown:
		r.editFocusBy(+1)
		return r, nil
	case k.Code == tea.KeyUp:
		r.editFocusBy(-1)
		return r, nil
	case k.Code == tea.KeyEnter:
		if r.edit.focus == editNote {
			return r.submitEdit()
		}
		r.editFocusBy(+1)
		return r, nil
	}

	var cmd tea.Cmd
	ed := r.edit
	switch ed.focus {
	case editVon:
		ed.von, cmd = ed.von.Update(k)
	case editBis:
		ed.bis, cmd = ed.bis.Update(k)
	case editTag:
		ed.tag, cmd = ed.tag.Update(k)
	case editNote:
		ed.note, cmd = ed.note.Update(k)
	}
	r.edit = ed
	return r, cmd
}

// editFocusBy moves focus by delta (±1) across [editVon..editNote], clamping at
// the edges instead of wrapping.
func (r *Route) editFocusBy(d int) {
	ed := r.edit
	r.blurEditField(ed)
	cur := int(ed.focus) + d
	if cur < int(editVon) {
		cur = int(editVon)
	}
	if cur > int(editNote) {
		cur = int(editNote)
	}
	ed.focus = editFocus(cur)
	r.edit = ed
	r.focusEditField()
}

func (r *Route) blurEditField(ed *editState) {
	switch ed.focus {
	case editVon:
		ed.von.Blur()
	case editBis:
		ed.bis.Blur()
	case editTag:
		ed.tag.Blur()
	case editNote:
		ed.note.Blur()
	}
}

func (r *Route) focusEditField() {
	ed := r.edit
	switch ed.focus {
	case editVon:
		_ = ed.von.Focus()
	case editBis:
		_ = ed.bis.Focus()
	case editTag:
		_ = ed.tag.Focus()
	case editNote:
		_ = ed.note.Focus()
	}
}

// submitEdit validates fields and calls api.EditSession. Modelled verbatim on
// Today's submitEdit: the dialog is NOT closed here — it is cleared only in the
// editDoneMsg success branch, so an error (e.g. overlap/409) keeps the dialog
// open and populated.
func (r *Route) submitEdit() (shell.Route, tea.Cmd) {
	ed := r.edit
	vonStr := strings.TrimSpace(ed.von.Value())
	bisStr := strings.TrimSpace(ed.bis.Value())
	tags := strings.Fields(ed.tag.Value())
	note := strings.TrimSpace(ed.note.Value())

	vonD, err := timefmt.ParseHM(vonStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r, r.toast.Init()
	}
	startTime := ed.date.Add(vonD)
	stopTime, err := timefmt.ParseStop(timefmt.NormalizeDurationArg(bisStr), startTime, time.Now())
	if err != nil {
		r.toast = toast.NewDanger("Stop ungültig", r.pal)
		return r, r.toast.Init()
	}

	id := ed.id
	api := r.api
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, editErr := api.EditSession(ctx, id, nil, &tags, note, startTime, &stopTime)
		if editErr != nil {
			return editDoneMsg{err: editErr}
		}
		return editDoneMsg{}
	}
}

// renderEdit renders the edit overlay into the given frame.
func (r *Route) renderEdit(f shell.Frame) string {
	ed := r.edit
	if ed == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n  Session bearbeiten  ")
	b.WriteString(theme.Dim("tab wechselt · enter speichert · esc bricht ab", f.Pal))
	b.WriteString("\n\n")
	labels := []string{"Von  ", "Bis  ", "Tag  ", "Notiz"}
	fields := []textinput.Model{ed.von, ed.bis, ed.tag, ed.note}
	for i, ti := range fields {
		fmt.Fprintf(&b, "  %s %s\n", labels[i], ti.View())
	}
	return b.String()
}

func editHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab/↑↓", Desc: "Feld"},
		{Key: "enter", Desc: "speichern"},
		{Key: "esc", Desc: "abbrechen"},
	}
}

// ---- Delete confirm --------------------------------------------------------

// delState holds the delete-confirm model plus the id targeted for deletion.
type delState struct {
	id    string
	model confirm.Model
}

// openDelete builds the danger confirm for the selected row.
func (r *Route) openDelete(row dayRow) {
	detail := row.Start.Format("15:04")
	if !row.Running {
		detail = fmt.Sprintf("%s → %s", row.Start.Format("15:04"), row.Stop.Format("15:04"))
	}
	r.del = &delState{
		id:    row.ID,
		model: confirm.NewDanger("Session löschen?", detail, r.pal),
	}
}

// delDoneMsg is sent when DeleteSession completes (or errors).
type delDoneMsg struct {
	err error
}

// deleteCmd issues the DeleteSession call for the confirmed id.
func (r *Route) deleteCmd(id string) tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.DeleteSession(ctx, id); err != nil {
			return delDoneMsg{err: err}
		}
		return delDoneMsg{}
	}
}

func deleteHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "y", Desc: "löschen"},
		{Key: "n", Desc: "abbrechen"},
	}
}
