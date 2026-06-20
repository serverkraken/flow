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
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
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
	target    string
	vonDP     datepicker.Model
	bisDP     datepicker.Model
	bisEdited bool // once true, Bis stops tracking Von
	label     textinput.Model
	addCur    int // 0=Von, 1=Bis, 2=Label
	confirm   confirm.Model
	blSel     int
}

func (r *Route) openTargetEdit() (shell.Route, tea.Cmd) {
	r.dialog = dialogTarget
	r.dlg.target = ""
	return r, nil
}

func (r *Route) openAdd() (shell.Route, tea.Cmd) {
	today := r.now()
	r.dlg.vonDP = datepicker.New(today, r.pal)
	r.dlg.bisDP = datepicker.New(today, r.pal)
	r.dlg.bisEdited = false
	r.dlg.label = form.NewTextInput("z.B. Urlaub", r.pal)
	r.dlg.vonDP.Focus()
	r.dlg.addCur = 0
	r.dialog = dialogAdd
	return r, nil
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
	case tea.KeyTab:
		r.addFocus(+1)
		return r, nil
	case tea.KeyEnter:
		if r.dlg.addCur == 2 {
			return r, r.submitAdd()
		}
		r.addFocus(+1)
		return r, nil
	}
	switch r.dlg.addCur {
	case 0: // Von — mirror into Bis while Bis is untouched
		if k.Text == "t" {
			_ = r.dlg.vonDP.SetValue(r.now().Format("2006-01-02"))
		} else {
			r.dlg.vonDP = r.dlg.vonDP.Update(k)
		}
		if !r.dlg.bisEdited {
			_ = r.dlg.bisDP.SetValue(r.dlg.vonDP.Value())
		}
	case 1: // Bis — latch bisEdited when the value actually changes
		if k.Text == "t" {
			_ = r.dlg.bisDP.SetValue(r.now().Format("2006-01-02"))
			r.dlg.bisEdited = true
		} else {
			before := r.dlg.bisDP.Value()
			r.dlg.bisDP = r.dlg.bisDP.Update(k)
			if r.dlg.bisDP.Value() != before {
				r.dlg.bisEdited = true
			}
		}
	case 2:
		var cmd tea.Cmd
		r.dlg.label, cmd = r.dlg.label.Update(k)
		return r, cmd
	}
	return r, nil
}

func (r *Route) addFocus(delta int) {
	r.dlg.addCur = (r.dlg.addCur + delta + 3) % 3
	r.dlg.vonDP.Blur()
	r.dlg.bisDP.Blur()
	r.dlg.label.Blur()
	switch r.dlg.addCur {
	case 0:
		r.dlg.vonDP.Focus()
	case 1:
		r.dlg.bisDP.Focus()
	case 2:
		_ = r.dlg.label.Focus()
	}
}

func (r *Route) submitAdd() tea.Cmd {
	from := r.dlg.vonDP.Value()
	to := r.dlg.bisDP.Value()
	if to < from { // ISO yyyy-mm-dd compares lexically
		return nil // keep dialog open; invalid range
	}
	label := strings.TrimSpace(r.dlg.label.Value())
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
	if cur, ok := listnav.New().Set(r.dlg.blSel, len(bundeslaender)).Handle(k, len(bundeslaender), 5); ok {
		r.dlg.blSel = cur.Index()
		return r, nil
	}
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
	case tea.KeyEnter:
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
		var b strings.Builder
		b.WriteString("\n  Frei-Tag anlegen (tab wechselt · t heute · enter speichert · esc ab)\n\n")
		fmt.Fprintf(&b, "  Von    %s\n", r.dlg.vonDP.View())
		fmt.Fprintf(&b, "  Bis    %s\n", r.dlg.bisDP.View())
		fmt.Fprintf(&b, "  Label  %s\n", r.dlg.label.View())
		switch r.dlg.addCur {
		case 0:
			b.WriteString("\n" + r.dlg.vonDP.Calendar(r.now()) + "\n")
		case 1:
			b.WriteString("\n" + r.dlg.bisDP.Calendar(r.now()) + "\n")
		}
		return b.String()
	case dialogDelete:
		return r.dlg.confirm.View()
	case dialogBundesland:
		var b strings.Builder
		b.WriteString("\n  Bundesland wählen (↑/↓ · enter · esc)\n\n")
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
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "t", Desc: "heute"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	case dialogBundesland:
		return []keyhint.Hint{{Key: "↑/↓", Desc: "wählen"}, {Key: "enter", Desc: "setzen"}, {Key: "esc", Desc: "abbrechen"}}
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
