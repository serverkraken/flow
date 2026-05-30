// Drill-mode session edit / add / delete dialogs. Sit on top of the
// History tab's day-detail drill so the user can correct sessions
// directly where they spotted them — e.g. "vergessen den Counter über
// Nacht auszuschalten" → open the day, edit the offending row's stop
// time. Mutations route through the same SessionWriter the Heute view
// uses (Edit / AddManual / Delete) so locking, overlap checks, and
// split-at-midnight stay in one place.

package worktime

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/usecase"
)

// — open helpers —

// openDrillEdit primes the edit form with the selected session's
// values. Activation requires the cursor sit on a real row;
// handleDrillKey gates that already, so the index access here is
// unconditional.
func (h history) openDrillEdit() (tea.Model, tea.Cmd) {
	s := h.drillSessions[h.drillCur]
	h.dialog = historyDialogDrillEdit
	h.drillEditIdx = h.drillCur
	h.drillForm = newSessionForm(
		h.pal,
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		s.Tag,
		s.Note,
	)
	h.drillFormCur = 0
	h.drillForm[0].Focus()
	h.errMsg = ""
	h.drillToast = nil
	return h, textinput.Blink
}

// openDrillAdd opens the form with empty fields. Default Start is
// the latest session's Stop (or 09:00 for an empty day) so the most
// common "add another session right after the previous one" workflow
// only requires the user to type the new Stop.
func (h history) openDrillAdd() (tea.Model, tea.Cmd) {
	startSeed, stopSeed := drillAddDefaults(h.drillSessions)
	h.dialog = historyDialogDrillAdd
	h.drillEditIdx = -1
	h.drillForm = newSessionForm(h.pal, startSeed, stopSeed, "", "")
	h.drillFormCur = 0
	h.drillForm[0].Focus()
	h.errMsg = ""
	h.drillToast = nil
	return h, textinput.Blink
}

// openDrillDelete shows the canonical confirm dialog. Submission
// triggers SessionWriter.Delete on the selected row.
func (h history) openDrillDelete() (tea.Model, tea.Cmd) {
	s := h.drillSessions[h.drillCur]
	h.drillEditIdx = h.drillCur
	h.dialog = historyDialogDrillDelete
	question := fmt.Sprintf("Session %d am %s löschen?",
		h.drillCur+1, h.drillDate.Format("2006-01-02"))
	detail := fmt.Sprintf("%s → %s   %s",
		s.Start.Format("15:04"), s.Stop.Format("15:04"), formatDur(s.Elapsed))
	cm := confirm.NewDanger(question, detail, h.pal)
	h.drillConfirm = &cm
	h.errMsg = ""
	h.drillToast = nil
	return h, cm.Init()
}

// — input handlers —

// handleDrillFormKey forwards keystrokes to the Edit / Add form.
// Tab / Shift-Tab / Up / Down move between fields; Enter advances
// until the last field, then submits.
func (h history) handleDrillFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	maxCur := len(h.drillForm) - 1
	switch msg.String() {
	case "esc":
		h.dialog = historyDialogDrill
		h.drillForm = nil
		h.drillFormCur = 0
		h.errMsg = ""
		return h, nil
	case "tab", "down":
		next := h.drillFormCur + 1
		if next > maxCur {
			next = 0
		}
		h.focusDrillForm(next)
		return h, textinput.Blink
	case "shift+tab", "up":
		next := h.drillFormCur - 1
		if next < 0 {
			next = maxCur
		}
		h.focusDrillForm(next)
		return h, textinput.Blink
	case "enter":
		if h.drillFormCur < maxCur {
			h.focusDrillForm(h.drillFormCur + 1)
			return h, textinput.Blink
		}
		return h.submitDrillForm()
	}
	h.errMsg = ""
	if h.drillFormCur >= 0 && h.drillFormCur < len(h.drillForm) {
		var cmd tea.Cmd
		h.drillForm[h.drillFormCur], cmd = h.drillForm[h.drillFormCur].Update(msg)
		return h, cmd
	}
	return h, nil
}

func (h *history) focusDrillForm(i int) {
	for j := range h.drillForm {
		if j == i {
			h.drillForm[j].Focus()
		} else {
			h.drillForm[j].Blur()
		}
	}
	h.drillFormCur = i
}

// handleDrillDeleteKey forwards to the canonical confirm.Model. The
// model emits a confirm.ResultMsg via tea.Cmd which the history's
// outer Update consumes — same pattern as today.go's delete dialog.
func (h history) handleDrillDeleteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if h.drillConfirm == nil {
		h.dialog = historyDialogDrill
		return h, nil
	}
	updated, cmd := h.drillConfirm.Update(msg)
	h.drillConfirm = &updated
	return h, cmd
}

// handleDrillConfirmResult resolves the drill-delete confirm dialog's
// ResultMsg: yes → dispatch Delete via the SessionWriter, no → close
// dialog without action. Mirrors today.go's confirm.ResultMsg branch.
func (h history) handleDrillConfirmResult(msg confirm.ResultMsg) (tea.Model, tea.Cmd) {
	if h.dialog != historyDialogDrillDelete {
		return h, nil
	}
	date := h.drillDate
	idx := h.drillEditIdx
	h.dialog = historyDialogDrill
	h.drillConfirm = nil
	if !msg.Confirmed {
		return h, nil
	}
	sw := h.deps.SessionWriter
	return h, func() tea.Msg {
		if err := sw.Delete(date, idx); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		return historyActionDoneMsg{
			toast: fmt.Sprintf("Session %d gelöscht", idx+1),
			date:  date,
		}
	}
}

// — submit —

// submitDrillForm validates the form, calls Edit or AddManual, and
// closes the dialog on success. Validation errors stay in errMsg and
// keep the dialog open so the user can correct. The two dispatch
// branches (add / edit) live in their own helpers so each can hold
// its async-tea.Cmd closure without tipping the gocognit budget.
func (h history) submitDrillForm() (tea.Model, tea.Cmd) {
	fields, ok := h.parseDrillFormFields()
	if !ok {
		// Validation error already lives in h.errMsg.
		return h, nil
	}
	date := h.drillDate
	if h.dialog == historyDialogDrillAdd {
		h.dialog = historyDialogDrill
		h.drillForm = nil
		h.drillFormCur = 0
		return h, dispatchDrillAdd(h.deps.SessionWriter, date, fields)
	}
	idx := h.drillEditIdx
	h.dialog = historyDialogDrill
	h.drillForm = nil
	h.drillFormCur = 0
	return h, dispatchDrillEdit(h.deps.SessionWriter, date, idx, fields)
}

// drillFormFields holds the parsed + validated drill-form values. Only
// produced by parseDrillFormFields; the dispatch helpers consume it
// without re-validating.
type drillFormFields struct {
	startTime time.Time
	stopTime  time.Time
	tag       string
	note      string
}

// parseDrillFormFields validates the textinputs into a typed struct.
// Returns ok=false on a validation error (and populates h.errMsg) so
// the dialog stays open for correction.
func (h *history) parseDrillFormFields() (drillFormFields, bool) {
	if len(h.drillForm) < 2 {
		return drillFormFields{}, false
	}
	startStr := strings.TrimSpace(h.drillForm[0].Value())
	stopStr := strings.TrimSpace(h.drillForm[1].Value())
	tag := strings.TrimSpace(h.drillForm[2].Value())
	note := strings.TrimSpace(h.drillForm[3].Value())

	startD, err := domain.ParseHM(startStr)
	if err != nil {
		h.errMsg = err.Error()
		return drillFormFields{}, false
	}
	base := startOfDay(h.drillDate)
	startTime := base.Add(startD)

	stopTime, err := parseDrillStop(stopStr, startTime, base)
	if err != nil {
		h.errMsg = err.Error()
		return drillFormFields{}, false
	}
	return drillFormFields{startTime: startTime, stopTime: stopTime, tag: tag, note: note}, true
}

// dispatchDrillAdd builds the tea.Cmd that appends a new session and
// — when present — applies tag/note to the freshly-appended row. Tag/
// Note setting takes a separate Lock pass; under writer concurrency
// the index lookup may target a row another writer just inserted, so
// the worst case is "tag landed on wrong session"; documented and
// accepted (see SessionWriter doc).
func dispatchDrillAdd(sw *usecase.SessionWriter, date time.Time, f drillFormFields) tea.Cmd {
	return func() tea.Msg {
		if err := sw.AddManual(date, f.startTime, f.stopTime); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		toast := fmt.Sprintf("Session am %s angelegt", date.Format("2006-01-02"))
		if f.tag == "" && f.note == "" {
			return historyActionDoneMsg{toast: toast, date: date}
		}
		all, err := sw.Sessions.LoadAll()
		if err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		idx := lastSessionIndexForDate(all, date)
		if idx < 0 {
			return historyActionDoneMsg{toast: toast, date: date}
		}
		if err := applyTagNote(sw, date, idx, f.tag, f.note); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		return historyActionDoneMsg{toast: toast, date: date}
	}
}

// dispatchDrillEdit builds the tea.Cmd that mutates an existing
// session at idx — Edit followed by SetTag / SetNote so empty-string
// inputs clear the slot.
func dispatchDrillEdit(sw *usecase.SessionWriter, date time.Time, idx int, f drillFormFields) tea.Cmd {
	return func() tea.Msg {
		if err := sw.Edit(date, idx, f.startTime, f.stopTime); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		if err := applyTagNote(sw, date, idx, f.tag, f.note); err != nil {
			return historyActionDoneMsg{err: err, date: date}
		}
		return historyActionDoneMsg{
			toast: fmt.Sprintf("Session %d aktualisiert", idx+1),
			date:  date,
		}
	}
}

// applyTagNote runs SetTag + SetNote on the targeted row. Empty inputs
// are passed through unchanged (the writer treats empty as „clear").
// Returns the first error encountered.
func applyTagNote(sw *usecase.SessionWriter, date time.Time, idx int, tag, note string) error {
	if err := sw.SetTag(date, idx, tag); err != nil {
		return err
	}
	if err := sw.SetNote(date, idx, note); err != nil {
		return err
	}
	return nil
}

// — render helpers —

// renderDrillDialog returns the dialog body rows for whichever drill
// dialog is active. Returns nil for plain-drill or no-dialog modes so
// the caller doesn't need to special-case.
func (h history) renderDrillDialog(inner int) []string {
	switch h.dialog {
	case historyDialogDrillEdit:
		return h.renderDrillFormDialog(inner, "session bearbeiten",
			fmt.Sprintf("Session %d", h.drillEditIdx+1))
	case historyDialogDrillAdd:
		return h.renderDrillFormDialog(inner, "neue session", "manueller Eintrag")
	case historyDialogDrillDelete:
		if h.drillConfirm == nil {
			return nil
		}
		return []string{
			picker.SectionHeader("löschen", inner, h.pal),
			"  " + h.drillConfirm.View(),
		}
	case historyDialogDrillNoteAttach:
		// notePicker.View liefert input + suggestion-list + "bereits
		// angehängt"-Hint + optionalen Error. Header wird hier
		// vorangestellt damit die Drill-Composition klar bleibt.
		return []string{
			picker.SectionHeader("Note an "+h.drillDate.Format("2006-01-02")+" anhängen", inner, h.pal),
			h.notePicker.View(inner),
		}
	}
	return nil
}

func (h history) renderDrillFormDialog(inner int, header, subtitle string) []string {
	rows := []string{picker.SectionHeader(header, inner, h.pal)}
	if subtitle != "" {
		rows = append(rows, stDim(h.pal, "  "+subtitle), "")
	}
	labels := []string{"Start (HH:MM)", "Stop (HH:MM oder +1h30m)", "Tag", "Notiz"}
	for i, ti := range h.drillForm {
		rows = append(rows, renderFormField(labels[i], ti, i == h.drillFormCur, inner, h.pal)...)
	}
	return rows
}

// — pure helpers (private) —

// newSessionForm builds the four-field [start, stop, tag, note] input
// stack used by both edit and add. Seed values are pre-filled so the
// edit case lands on a fully-populated form and the add case can
// inherit a sensible default Start.
func newSessionForm(pal theme.Palette, startVal, stopVal, tagVal, noteVal string) []textinput.Model {
	start := form.NewTextInput("HH:MM", pal)
	start.SetValue(startVal)
	stop := form.NewTextInput("HH:MM oder +1h30m", pal)
	stop.SetValue(stopVal)
	tag := form.NewTextInput("z.B. deep, meeting", pal)
	tag.SetValue(tagVal)
	note := form.NewTextInput("kurzer Text", pal)
	note.SetValue(noteVal)
	return []textinput.Model{start, stop, tag, note}
}

// drillAddDefaults picks reasonable Start / Stop seeds for a new
// session: Start = last existing session's Stop (or 09:00 for an
// empty day) — stop is left blank so the user picks. The most common
// "add another session right after this one" flow becomes one input.
func drillAddDefaults(sessions []domain.Session) (string, string) {
	if len(sessions) == 0 {
		return "09:00", ""
	}
	last := sessions[len(sessions)-1]
	return last.Stop.Format("15:04"), ""
}

// lastSessionIndexForDate returns the 0-based index (within the
// per-day session list) of the last session for date in the full log.
// Used by the AddManual + SetTag/SetNote sequence to land tag/note on
// the row we just appended. Returns -1 when no session exists for
// that day — a possible race where another writer rewrote the log
// between AddManual and the lookup.
func lastSessionIndexForDate(all []domain.Session, date time.Time) int {
	dateStr := date.Format("2006-01-02")
	idx := -1
	dayIdx := 0
	for _, s := range all {
		if s.Date.Format("2006-01-02") == dateStr {
			idx = dayIdx
			dayIdx++
		}
	}
	return idx
}

// parseDrillStop parses a Stop input for a drill-edit / drill-add
// dialog. Two accepted shapes:
//
//   - "HH:MM"     → that time on the drill date (NOT today). domain.
//     ParseStop anchors HH:MM on `now` and rejects "Zeit liegt in der
//     Zukunft" if the past day's stop hasn't happened in real time
//     yet — wrong here, the user is editing a past session.
//   - "+1h30m"    → start + duration. Case-insensitive (`+8H02M` is
//     accepted as `+8h02m`); domain.parseHumanDuration is strict-lower.
//
// Empty input is rejected — the drill always needs an explicit stop.
func parseDrillStop(arg string, start, base time.Time) (time.Time, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return time.Time{}, fmt.Errorf("stoppzeit darf nicht leer sein")
	}
	if arg[0] == '+' {
		return domain.ParseStop(normalizeDurationArg(arg), start, time.Time{})
	}
	hm, err := domain.ParseHM(arg)
	if err != nil {
		return time.Time{}, err
	}
	return base.Add(hm), nil
}
