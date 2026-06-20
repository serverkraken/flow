package worktime

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

type todayAPI interface {
	GetToday(context.Context) (apiclient.Today, error)
	ListSessions(context.Context) ([]domain.WorkSession, error)
	ListSessionsSince(context.Context, time.Time) ([]domain.WorkSession, error)
	ListProjects(context.Context) ([]domain.Project, error)
	StartSession(context.Context, *string, string, string) (domain.WorkSession, error)
	StopSession(ctx context.Context, id, projectID string) (domain.WorkSession, error)
	EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	DeleteSession(ctx context.Context, id string) error
	CreateProject(ctx context.Context, name string) (domain.Project, error)
}

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogBooking
	dialogEdit
	dialogDelete
)

type loadedMsg struct {
	today    apiclient.Today
	sessions []domain.WorkSession
	projects []domain.Project
	err      error
}
type projectsMsg struct{ projects []domain.Project }
type liveTickMsg struct{}

type TodayRoute struct {
	api todayAPI
	now func() time.Time
	pal theme.Palette
	reg wtnav.Registry // lateral sibling navigation

	st      todayState
	cursor  int
	loaded  bool
	ticking bool
	err     error
	toast   toast.Model

	dialog  dialogKind
	booking bookingState
	edit    editState
	confirm confirmState
}

func NewTodayRoute(api todayAPI, now func() time.Time, pal theme.Palette, reg wtnav.Registry) *TodayRoute {
	if now == nil {
		now = time.Now
	}
	return &TodayRoute{api: api, now: now, pal: pal, reg: reg}
}

func (r *TodayRoute) Title() string { return "Worktime" }

func (r *TodayRoute) Init() tea.Cmd {
	// Fresh (re)entry: any prior tick loop has already stopped (its ticks now
	// route to whichever tab is active), so clear the guard to let loadedMsg
	// restart the live tick.
	r.ticking = false
	return r.loadCmd()
}

func (r *TodayRoute) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := api.GetToday(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		sessions, err := api.ListSessions(ctx)
		if err != nil {
			return loadedMsg{err: err}
		}
		// Projects resolve each session's ProjectID to a display name; a
		// failure here only drops the names, not the session list.
		projects, _ := api.ListProjects(ctx)
		return loadedMsg{today: today, sessions: sessions, projects: projects}
	}
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return liveTickMsg{} })
}

func (r *TodayRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.err = m.err
		if m.err == nil {
			r.st = reconstruct(m.today, m.sessions, m.projects, r.now())
			if r.cursor >= len(r.st.Completed) {
				r.cursor = max(0, len(r.st.Completed)-1)
			}
		}
		if r.st.Running && !r.ticking {
			r.ticking = true
			return r, liveTickCmd()
		}
		return r, nil
	case liveTickMsg:
		if r.st.Running {
			return r, liveTickCmd()
		}
		r.ticking = false
		return r, nil
	case projectsMsg:
		r.booking.list = r.booking.list.SetItems(projectItems(m.projects))
		return r, nil
	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case reloadMsg:
		return r, r.loadCmd()
	case confirm.ResultMsg:
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.cursor < len(r.st.Completed) {
			id := r.st.Completed[r.cursor].ID
			api := r.api
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := api.DeleteSession(ctx, id); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}

func (r *TodayRoute) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k)
	}
	switch {
	case k.Text == "j" || k.Code == tea.KeyDown:
		r.applyKey("j")
	case k.Text == "k" || k.Code == tea.KeyUp:
		r.applyKey("k")
	case k.Text == "g":
		r.cursor = 0
	case k.Text == "G":
		r.cursor = max(0, len(r.st.Completed)-1)
	case k.Text == "w" || k.Text == "t" || k.Text == "d" || k.Text == "e":
		return r, r.reg.Nav(k.Text)
	case k.Text == "s":
		return r.startOrStop()
	case k.Text == "E" || k.Code == tea.KeyEnter:
		return r.openEdit()
	case k.Text == "D":
		return r.openDelete()
	}
	return r, nil
}

func (r *TodayRoute) applyKey(key string) {
	n := len(r.st.Completed)
	if n == 0 {
		r.cursor = 0
		return
	}
	switch key {
	case "j":
		r.cursor = (r.cursor + 1) % n
	case "k":
		r.cursor = (r.cursor + n - 1) % n
	}
}

func (r *TodayRoute) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Heute lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return r.renderDialog(f)
	}
	return renderBody(r.st, r.cursor, f.Width, f.Height, r.now(), &r.toast, f.Pal)
}

// CapturesInput reports that Today owns the keyboard while a dialog is open, so
// the Shell forwards digits/Tab/Esc/etc. to the dialog instead of treating them
// as global shortcuts. Implements shell.InputCapturer.
func (r *TodayRoute) CapturesInput() bool { return r.dialog != dialogNone }

func (r *TodayRoute) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints()
	}
	hints := []keyhint.Hint{}
	if r.st.Running {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "stoppen"})
	} else {
		hints = append(hints, keyhint.Hint{Key: "s", Desc: "starten"})
	}
	hints = append(hints, keyhint.Hint{Key: "j/k", Desc: "bewegen"})
	if len(r.st.Completed) > 0 {
		hints = append(hints, keyhint.Hint{Key: "enter", Desc: "bearbeiten"})
	}
	hints = append(hints, keyhint.Hint{Key: "?", Desc: "Hilfe"})
	if len(hints) > 4 {
		hints = hints[:4]
	}
	return hints
}
