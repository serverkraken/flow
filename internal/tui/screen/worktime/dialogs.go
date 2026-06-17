package worktime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

type bookingState struct {
	projects []domain.Project
	sel      int
	newName  string
}

type reloadMsg struct{}

func (r *TodayRoute) startOrStop() (shell.Route, tea.Cmd) {
	if !r.st.Running {
		api := r.api
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StartSession(ctx, nil, "", ""); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	}
	r.dialog = dialogBooking
	r.booking = bookingState{}
	api := r.api
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, _ := api.ListProjects(ctx)
		return projectsMsg{projects: ps}
	}
}

func (r *TodayRoute) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogBooking:
		return r.handleBookingKey(k)
	case dialogEdit:
		return r.handleEditKey(k)
	case dialogDelete:
		m, cmd := r.confirm.model.Update(k)
		r.confirm.model = m
		return r, cmd
	}
	return r, nil
}

func (r *TodayRoute) handleBookingKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyEnter:
		id := r.st.ActiveID
		name := strings.TrimSpace(r.booking.newName)
		r.dialog = dialogNone
		api := r.api
		if name != "" {
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				p, err := api.CreateProject(ctx, name)
				if err != nil {
					return loadedMsg{err: err}
				}
				if _, err := api.StopSession(ctx, id, p.ID); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		if len(r.booking.projects) == 0 {
			r.dialog = dialogBooking
			r.toast = toast.NewDanger("Keine Projekte – tippe einen Namen ein oder warte", r.pal)
			return r, r.toast.Init()
		}
		if r.booking.sel >= len(r.booking.projects) {
			r.booking.sel = 0
		}
		pid := r.booking.projects[r.booking.sel].ID
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StopSession(ctx, id, pid); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	case k.Code == tea.KeyBackspace:
		if rn := []rune(r.booking.newName); len(rn) > 0 {
			r.booking.newName = string(rn[:len(rn)-1])
		}
	case k.Text == "j" && r.booking.newName == "":
		if r.booking.sel < len(r.booking.projects)-1 {
			r.booking.sel++
		}
	case k.Text == "k" && r.booking.newName == "":
		if r.booking.sel > 0 {
			r.booking.sel--
		}
	case k.Text != "":
		r.booking.newName += k.Text
	}
	return r, nil
}

type editState struct {
	id   string
	date time.Time
	form []textinput.Model
	cur  int
}

func (r *TodayRoute) openEdit() (shell.Route, tea.Cmd) {
	if r.cursor >= len(r.st.Completed) {
		return r, nil
	}
	s := r.st.Completed[r.cursor]
	start := form.NewTextInput("HH:MM", r.pal)
	start.SetValue(s.Start.Format("15:04"))
	stop := form.NewTextInput("HH:MM oder +1h30m", r.pal)
	stop.SetValue(s.Stop.Format("15:04"))
	tag := form.NewTextInput("z.B. deep, meeting", r.pal)
	tag.SetValue(s.Tag)
	note := form.NewTextInput("kurzer Text", r.pal)
	note.SetValue(s.Note)
	cmd := start.Focus()
	r.edit = editState{id: s.ID, date: s.Start, form: []textinput.Model{start, stop, tag, note}, cur: 0}
	r.dialog = dialogEdit
	return r, cmd
}

func (r *TodayRoute) handleEditKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		r.editFocus(-1)
		return r, nil
	case k.Code == tea.KeyTab || k.Code == tea.KeyDown:
		r.editFocus(+1)
		return r, nil
	case k.Code == tea.KeyUp:
		r.editFocus(-1)
		return r, nil
	case k.Code == tea.KeyEnter:
		if r.edit.cur == len(r.edit.form)-1 {
			return r, r.submitEdit()
		}
		r.editFocus(+1)
		return r, nil
	}
	var cmd tea.Cmd
	r.edit.form[r.edit.cur], cmd = r.edit.form[r.edit.cur].Update(k)
	return r, cmd
}

func (r *TodayRoute) editFocus(d int) {
	r.edit.form[r.edit.cur].Blur()
	n := len(r.edit.form)
	r.edit.cur = (r.edit.cur + d + n) % n
	_ = r.edit.form[r.edit.cur].Focus()
}

func (r *TodayRoute) submitEdit() tea.Cmd {
	startStr := strings.TrimSpace(r.edit.form[0].Value())
	stopStr := strings.TrimSpace(r.edit.form[1].Value())
	tag := strings.TrimSpace(r.edit.form[2].Value())
	note := strings.TrimSpace(r.edit.form[3].Value())
	startD, err := parseHM(startStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r.toast.Init()
	}
	base := time.Date(r.edit.date.Year(), r.edit.date.Month(), r.edit.date.Day(), 0, 0, 0, 0, r.edit.date.Location())
	startTime := base.Add(startD)
	stopTime, err := parseStop(normalizeDurationArg(stopStr), startTime, r.now())
	if err != nil {
		r.toast = toast.NewDanger("Stop ungültig", r.pal)
		return r.toast.Init()
	}
	id := r.edit.id
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := api.EditSession(ctx, id, nil, tag, note, startTime, &stopTime); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

type confirmState struct{ model confirm.Model }

func (r *TodayRoute) openDelete() (shell.Route, tea.Cmd) {
	if r.cursor >= len(r.st.Completed) {
		return r, nil
	}
	s := r.st.Completed[r.cursor]
	r.confirm = confirmState{model: confirm.NewDanger(
		"Session löschen?",
		fmt.Sprintf("%s → %s", s.Start.Format("15:04"), s.Stop.Format("15:04")), r.pal)}
	r.dialog = dialogDelete
	return r, nil
}

func (r *TodayRoute) renderDialog(f shell.Frame) string {
	switch r.dialog {
	case dialogBooking:
		return r.renderBooking(f)
	case dialogEdit:
		return r.renderEdit(f)
	case dialogDelete:
		return r.confirm.model.View()
	}
	return ""
}

func (r *TodayRoute) renderBooking(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Projekt buchen (j/k wählen · tippen = neu · enter)\n\n")
	if r.booking.newName != "" {
		b.WriteString("  neu: " + r.booking.newName + "\n")
	} else {
		for i, p := range r.booking.projects {
			b.WriteString(picker.Row(i == r.booking.sel, p.Name, "", f.Width-4, f.Pal) + "\n")
		}
	}
	return b.String()
}

func (r *TodayRoute) renderEdit(f shell.Frame) string {
	labels := []string{"Start", "Stop", "Tag", "Note"}
	var b strings.Builder
	b.WriteString("\n  Session bearbeiten (tab wechselt · enter speichert · esc bricht ab)\n\n")
	for i, ti := range r.edit.form {
		fmt.Fprintf(&b, "  %-6s %s\n", labels[i], ti.View())
	}
	return b.String()
}

func (r *TodayRoute) dialogHints() []keyhint.Hint {
	switch r.dialog {
	case dialogBooking:
		return []keyhint.Hint{{Key: "j/k", Desc: "wählen"}, {Key: "enter", Desc: "buchen"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogEdit:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	}
	return nil
}
