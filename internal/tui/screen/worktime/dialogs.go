package worktime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

type bookingState struct {
	list fuzzylist.Model
}

func engagementItems(nodes []domain.Node) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, fuzzylist.Item{ID: n.ID, Label: n.Name})
	}
	return out
}

type reloadMsg struct{}

func (r *TodayRoute) startOrStop() (shell.Route, tea.Cmd) {
	if !r.st.Running {
		api := r.api
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StartSession(ctx, nil, nil, ""); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	}
	r.dialog = dialogBooking
	r.booking = bookingState{list: fuzzylist.New(nil, r.pal).WithCreateHint("neu: %s")}
	api := r.api
	since := r.now().AddDate(0, 0, -90)
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, _ := api.ListNodes(ctx)
		ss, _ := api.ListSessionsSince(ctx, since)
		return projectsMsg{projects: mruEngagements(ps, ss)}
	}
}

func (r *TodayRoute) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogBooking:
		return r.handleBookingKey(k)
	case dialogEdit:
		return r.handleEditKey(k)
	case dialogEditStart:
		return r.handleAdjustStartKey(k)
	case dialogDelete:
		m, cmd := r.confirm.model.Update(k)
		r.confirm.model = m
		return r, cmd
	}
	return r, nil
}

func (r *TodayRoute) handleBookingKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyEnter:
		it, isCreate, ok := r.booking.list.Selection()
		if !ok {
			return r, nil
		}
		id := r.st.ActiveID
		api := r.api
		r.dialog = dialogNone
		if isCreate {
			name := strings.TrimSpace(r.booking.list.Query())
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				n, err := api.CreateNode(ctx, apiclient.CreateNodeFields{Name: name, Kind: string(domain.KindEngagement)})
				if err != nil {
					return loadedMsg{err: err}
				}
				if _, err := api.StopSession(ctx, id, n.ID); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		pid := it.ID
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StopSession(ctx, id, pid); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	default:
		r.booking.list = r.booking.list.Update(k)
		return r, nil
	}
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
	tag := form.NewTextInput("z.B. deep meeting (Leerzeichen trennt)", r.pal)
	tag.SetValue(strings.Join(s.Tags, " "))
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
	tags := strings.Fields(r.edit.form[2].Value())
	note := strings.TrimSpace(r.edit.form[3].Value())
	startD, err := wtfmt.ParseHM(startStr)
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r.toast.Init()
	}
	base := time.Date(r.edit.date.Year(), r.edit.date.Month(), r.edit.date.Day(), 0, 0, 0, 0, r.edit.date.Location())
	startTime := base.Add(startD)
	stopTime, err := wtfmt.ParseStop(wtfmt.NormalizeDurationArg(stopStr), startTime, r.now())
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
		if _, err := api.EditSession(ctx, id, nil, &tags, note, startTime, &stopTime); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

type adjustState struct {
	id    string
	date  time.Time
	input textinput.Model
}

// openAdjustStart opens the start-edit dialog for the *running* session,
// prefilled with its current start time. Reached via the ":" palette entry.
func (r *TodayRoute) openAdjustStart() (shell.Route, tea.Cmd) {
	if !r.st.Running || r.st.Active == nil {
		return r, nil
	}
	in := form.NewTextInput("HH:MM", r.pal)
	in.SetValue(r.st.Active.Format("15:04"))
	cmd := in.Focus()
	r.adjust = adjustState{id: r.st.ActiveID, date: *r.st.Active, input: in}
	r.dialog = dialogEditStart
	return r, cmd
}

func (r *TodayRoute) handleAdjustStartKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyEnter:
		return r, r.submitAdjustStart()
	}
	var cmd tea.Cmd
	r.adjust.input, cmd = r.adjust.input.Update(k)
	return r, cmd
}

// submitAdjustStart validates the HH:MM field and, on success, edits the
// running session's start time with stop=nil so it keeps running.
func (r *TodayRoute) submitAdjustStart() tea.Cmd {
	startD, err := wtfmt.ParseHM(strings.TrimSpace(r.adjust.input.Value()))
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r.toast.Init()
	}
	d := r.adjust.date
	base := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
	startTime := base.Add(startD)
	if startTime.After(r.now()) {
		r.toast = toast.NewDanger("Start liegt in der Zukunft", r.pal)
		return r.toast.Init()
	}
	id := r.adjust.id
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := api.EditSession(ctx, id, nil, nil, "", startTime, nil); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *TodayRoute) renderAdjustStart(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Startzeit anpassen (enter speichert · esc bricht ab)\n\n")
	fmt.Fprintf(&b, "  %-6s %s\n", "Start", r.adjust.input.View())
	return b.String()
}

type confirmState struct{ model confirm.Model }

func (r *TodayRoute) openDelete() (shell.Route, tea.Cmd) {
	if r.cursor >= len(r.st.Completed) {
		return r, nil
	}
	s := r.st.Completed[r.cursor]
	r.confirm = confirmState{model: confirm.NewDanger(
		"Session löschen?",
		fmt.Sprintf("%s → %s", s.Start.Format("15:04"), s.Stop.Format("15:04")), r.pal,
	)}
	r.dialog = dialogDelete
	return r, nil
}

func (r *TodayRoute) renderDialog(f shell.Frame) string {
	switch r.dialog {
	case dialogBooking:
		return r.renderBooking(f)
	case dialogEdit:
		return r.renderEdit(f)
	case dialogEditStart:
		return r.renderAdjustStart(f)
	case dialogDelete:
		return r.confirm.model.View()
	}
	return ""
}

func (r *TodayRoute) renderBooking(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Engagement buchen / wählen  ")
	b.WriteString(theme.Dim("tippen → filtern  ·  ↑/↓ → wählen  ·  enter → buchen  ·  esc", f.Pal))
	b.WriteString("\n\n")
	b.WriteString(r.booking.list.View(f.Width - 4))
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
		return []keyhint.Hint{{Key: "↑/↓", Desc: "wählen"}, {Key: "enter", Desc: "buchen"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogEdit:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogEditStart:
		return []keyhint.Hint{{Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	}
	return nil
}
