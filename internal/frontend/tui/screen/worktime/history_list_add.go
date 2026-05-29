package worktime

// History list-level add dialog — "nachbuchen" für Tage, an denen der
// User vergessen hat den Timer zu starten. Öffnet per `a` in der List-
// Ansicht ein 5-Feld-Formular (Datum → Start → Stop → Tag → Note),
// seeded auf gestern/letzten Werktag. Mutation via SessionWriter.AddManual,
// identisch zum Drill-Add (history_edit.go).

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func (h history) openListAdd() (tea.Model, tea.Cmd) {
	h.dialog = historyDialogListAdd
	h.listAddForm = newListAddForm(h.pal, lastWorkday(h.deps.Clock.Now()))
	h.listAddFormCur = 0
	h.listAddForm[0].Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h history) handleListAddKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	maxCur := len(h.listAddForm) - 1
	switch msg.String() {
	case "esc":
		h.dialog = historyDialogNone
		h.listAddForm = nil
		h.listAddFormCur = 0
		h.errMsg = ""
		return h, nil
	case "tab", "down":
		next := h.listAddFormCur + 1
		if next > maxCur {
			next = 0
		}
		h.focusListAddForm(next)
		return h, textinput.Blink
	case "shift+tab", "up":
		next := h.listAddFormCur - 1
		if next < 0 {
			next = maxCur
		}
		h.focusListAddForm(next)
		return h, textinput.Blink
	case "enter":
		if h.listAddFormCur < maxCur {
			h.focusListAddForm(h.listAddFormCur + 1)
			return h, textinput.Blink
		}
		return h.submitListAddForm()
	}
	h.errMsg = ""
	if h.listAddFormCur >= 0 && h.listAddFormCur < len(h.listAddForm) {
		var cmd tea.Cmd
		h.listAddForm[h.listAddFormCur], cmd = h.listAddForm[h.listAddFormCur].Update(msg)
		return h, cmd
	}
	return h, nil
}

func (h *history) focusListAddForm(i int) {
	for j := range h.listAddForm {
		if j == i {
			h.listAddForm[j].Focus()
		} else {
			h.listAddForm[j].Blur()
		}
	}
	h.listAddFormCur = i
}

func (h history) submitListAddForm() (tea.Model, tea.Cmd) {
	if len(h.listAddForm) < 5 {
		return h, nil
	}
	dateStr := strings.TrimSpace(h.listAddForm[0].Value())
	startStr := strings.TrimSpace(h.listAddForm[1].Value())
	stopStr := strings.TrimSpace(h.listAddForm[2].Value())
	tag := strings.TrimSpace(h.listAddForm[3].Value())
	note := strings.TrimSpace(h.listAddForm[4].Value())

	date, err := time.ParseInLocation("2006-01-02", dateStr, h.deps.Clock.Now().Location())
	if err != nil {
		h.errMsg = "ungültiges Datum — Format: YYYY-MM-DD"
		return h, nil
	}
	if date.After(startOfDay(h.deps.Clock.Now())) {
		h.errMsg = "Datum darf nicht in der Zukunft liegen"
		return h, nil
	}

	startD, err := domain.ParseHM(startStr)
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}
	base := startOfDay(date)
	startTime := base.Add(startD)

	stopTime, err := parseDrillStop(stopStr, startTime, base)
	if err != nil {
		h.errMsg = err.Error()
		return h, nil
	}

	h.dialog = historyDialogNone
	h.listAddForm = nil
	h.listAddFormCur = 0
	h.errMsg = ""

	f := drillFormFields{startTime: startTime, stopTime: stopTime, tag: tag, note: note}
	return h, dispatchDrillAdd(h.deps.SessionWriter, date, f)
}

func (h history) renderListAddDialog() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{
		picker.SectionHeader("session nachbuchen", inner, h.pal),
		"",
	}
	labels := []string{"Datum (YYYY-MM-DD)", "Start (HH:MM)", "Stop (HH:MM oder +1h30m)", "Tag", "Notiz"}
	for i, ti := range h.listAddForm {
		rows = append(rows, renderFormField(labels[i], ti, i == h.listAddFormCur, inner, h.pal)...)
	}
	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	rows = append(rows, "", stDim(h.pal, "  "+uistrings.HintFormNav))
	return strings.Join(rows, "\n")
}

func newListAddForm(pal theme.Palette, seedDate time.Time) []textinput.Model {
	date := form.NewTextInput("YYYY-MM-DD", pal)
	date.SetValue(seedDate.Format("2006-01-02"))
	start := form.NewTextInput("HH:MM", pal)
	start.SetValue("09:00")
	stop := form.NewTextInput("HH:MM oder +1h30m", pal)
	tag := form.NewTextInput("z.B. deep, meeting", pal)
	note := form.NewTextInput("kurzer Text", pal)
	return []textinput.Model{date, start, stop, tag, note}
}

// lastWorkday returns yesterday, or Friday if yesterday is a weekend day.
func lastWorkday(now time.Time) time.Time {
	y := now.AddDate(0, 0, -1)
	switch y.Weekday() {
	case time.Sunday:
		return y.AddDate(0, 0, -2)
	case time.Saturday:
		return y.AddDate(0, 0, -1)
	}
	return y
}
