package dayoffs

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogTarget
	dialogAdd
	dialogDelete
	dialogBundesland
)

var bundeslaender = []string{
	"BW", "BY", "BE", "BB", "HB", "HH", "HE", "MV",
	"NI", "NW", "RP", "SL", "SN", "ST", "SH", "TH", "",
}

type dialogState struct {
	target  string
	addForm []textinput.Model
	addCur  int
	confirm confirm.Model
	blSel   int
}

func (r *Route) openTargetEdit() (shell.Route, tea.Cmd) {
	r.dialog = dialogTarget
	r.dlg.target = ""
	return r, nil
}

func (r *Route) openAdd() (shell.Route, tea.Cmd) {
	from := form.NewTextInput("YYYY-MM-DD", r.pal)
	to := form.NewTextInput("YYYY-MM-DD (leer = wie von)", r.pal)
	label := form.NewTextInput("z.B. Urlaub", r.pal)
	cmd := from.Focus()
	r.dlg.addForm = []textinput.Model{from, to, label}
	r.dlg.addCur = 0
	r.dialog = dialogAdd
	return r, cmd
}

func (r *Route) openDelete() (shell.Route, tea.Cmd) {
	if r.cursor >= len(r.list) {
		return r, nil
	}
	d := r.list[r.cursor]
	r.dlg.confirm = confirm.NewDanger("Frei-Tag löschen?", d.Day+" "+d.Label, r.pal)
	r.dialog = dialogDelete
	return r, nil
}

func (r *Route) openBundesland() (shell.Route, tea.Cmd) {
	r.dlg.blSel = 0
	for i, b := range bundeslaender {
		if b == r.settings.Bundesland {
			r.dlg.blSel = i
		}
	}
	r.dialog = dialogBundesland
	return r, nil
}

func (r *Route) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogTarget:
		return r.handleTargetKey(k)
	case dialogAdd:
		return r.handleAddKey(k)
	case dialogDelete:
		m, cmd := r.dlg.confirm.Update(k)
		r.dlg.confirm = m
		return r, cmd
	case dialogBundesland:
		return r.handleBundeslandKey(k)
	}
	return r, nil
}

func (r *Route) handleTargetKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
	case k.Code == tea.KeyEnter:
		mins := parseDigits(r.dlg.target)
		if mins <= 0 {
			return r, nil
		}
		r.dialog = dialogNone
		return r, r.setTargetCmd(mins)
	case k.Code == tea.KeyBackspace:
		if rn := []rune(r.dlg.target); len(rn) > 0 {
			r.dlg.target = string(rn[:len(rn)-1])
		}
	case k.Text != "" && unicode.IsDigit([]rune(k.Text)[0]):
		r.dlg.target += k.Text
	}
	return r, nil
}

func (r *Route) handleAddKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyTab, tea.KeyDown:
		r.addFocus(+1)
		return r, nil
	case tea.KeyUp:
		r.addFocus(-1)
		return r, nil
	case tea.KeyEnter:
		if r.dlg.addCur == len(r.dlg.addForm)-1 {
			return r, r.submitAdd()
		}
		r.addFocus(+1)
		return r, nil
	}
	var cmd tea.Cmd
	r.dlg.addForm[r.dlg.addCur], cmd = r.dlg.addForm[r.dlg.addCur].Update(k)
	return r, cmd
}

func (r *Route) addFocus(d int) {
	r.dlg.addForm[r.dlg.addCur].Blur()
	n := len(r.dlg.addForm)
	r.dlg.addCur = (r.dlg.addCur + d + n) % n
	// Focus() returns a cursor-blink cmd; intentionally discarded — the dialog re-renders on the next key.
	_ = r.dlg.addForm[r.dlg.addCur].Focus()
}

func (r *Route) submitAdd() tea.Cmd {
	from := strings.TrimSpace(r.dlg.addForm[0].Value())
	to := strings.TrimSpace(r.dlg.addForm[1].Value())
	label := strings.TrimSpace(r.dlg.addForm[2].Value())
	if from == "" {
		return nil
	}
	if to == "" {
		to = from
	}
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.AddDayOffs(ctx, from, to, "urlaub", label, 0, true); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) handleBundeslandKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
	case k.Text == "j" || k.Code == tea.KeyDown:
		if r.dlg.blSel < len(bundeslaender)-1 {
			r.dlg.blSel++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if r.dlg.blSel > 0 {
			r.dlg.blSel--
		}
	case k.Code == tea.KeyEnter:
		land := bundeslaender[r.dlg.blSel]
		r.dialog = dialogNone
		return r, r.setBundeslandCmd(land)
	}
	return r, nil
}

func (r *Route) setTargetCmd(defaultMin int) tea.Cmd {
	api := r.api
	weekday := r.settings.WeekdayTargetMin
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.SetTargetConfig(ctx, defaultMin, weekday); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) deleteCmd(day string) tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.DeleteDayOff(ctx, day); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) setBundeslandCmd(land string) tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.SetBundesland(ctx, land); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *Route) renderDialog(f shell.Frame) string {
	switch r.dialog {
	case dialogTarget:
		return "\n  Neues Tagesziel (Minuten): " + r.dlg.target + "▏\n  Ziffern · enter ok · esc ab\n"
	case dialogAdd:
		labels := []string{"Von", "Bis", "Label"}
		var b strings.Builder
		b.WriteString("\n  Frei-Tag anlegen (tab wechselt · enter speichert · esc ab)\n\n")
		for i, ti := range r.dlg.addForm {
			fmt.Fprintf(&b, "  %-6s %s\n", labels[i], ti.View())
		}
		return b.String()
	case dialogDelete:
		return r.dlg.confirm.View()
	case dialogBundesland:
		var b strings.Builder
		b.WriteString("\n  Bundesland wählen (j/k · enter · esc)\n\n")
		for i, land := range bundeslaender {
			label := land
			if label == "" {
				label = "(aus)"
			}
			b.WriteString(picker.Row(i == r.dlg.blSel, label, "", f.Width-4, f.Pal) + "\n")
		}
		return b.String()
	}
	return ""
}

func (r *Route) dialogHints() []keyhint.Hint {
	switch r.dialog {
	case dialogTarget:
		return []keyhint.Hint{{Key: "enter", Desc: "ok"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogAdd:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	case dialogBundesland:
		return []keyhint.Hint{{Key: "j/k", Desc: "wählen"}, {Key: "enter", Desc: "setzen"}, {Key: "esc", Desc: "abbrechen"}}
	}
	return nil
}

func parseDigits(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
