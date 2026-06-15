// Package tui is the flow terminal UI (Bubbletea v2). M1a ships one screen:
// the worktime timer, live-synced via the server SSE stream.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// Model is the worktime screen (and, for M1a, the whole app shell).
type Model struct {
	client *apiclient.Client
	user   string

	sessions []domain.WorkSession
	projects []domain.Project
	running  *domain.WorkSession
	now      time.Time

	booking bool
	sel     int
	newName string

	status string
	err    error
	events <-chan apiclient.ClientEvent

	showDayOffs bool
	dayoffs     []apiclient.DayOff

	today    apiclient.Today
	burndown apiclient.Burndown
}

// New builds the model. client may be nil in tests that only drive Update.
func New(client *apiclient.Client, user string) Model {
	return Model{client: client, user: user, now: time.Now()}
}

type loadedMsg struct {
	sessions []domain.WorkSession
	projects []domain.Project
	now      time.Time
}
type statsLoadedMsg struct {
	today    apiclient.Today
	burndown apiclient.Burndown
}
type eventMsg struct{ ev apiclient.ClientEvent }
type eventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }
type tickMsg time.Time
type errMsg struct{ err error }

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.subscribe(), m.reloadStats(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) reload() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sessions, err := m.client.ListSessions(ctx)
		if err != nil {
			return errMsg{err}
		}
		projects, err := m.client.ListProjects(ctx)
		if err != nil {
			return errMsg{err}
		}
		return loadedMsg{sessions: sessions, projects: projects, now: time.Now()}
	}
}

func (m Model) reloadStats() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := m.client.GetToday(ctx)
		if err != nil {
			return errMsg{err}
		}
		bd, err := m.client.GetBurndown(ctx)
		if err != nil {
			return errMsg{err}
		}
		return statsLoadedMsg{today: today, burndown: bd}
	}
}

func (m Model) subscribe() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ch, err := m.client.Events(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return eventsReadyMsg{ch}
	}
}

func waitForEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

func (m Model) startCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.client.StartSession(ctx, nil, "", ""); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) stopCmd(sessionID, projectID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.client.StopSession(ctx, sessionID, projectID); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) createAndStopCmd(sessionID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, err := m.client.CreateProject(ctx, name)
		if err != nil {
			return errMsg{err}
		}
		if _, err := m.client.StopSession(ctx, sessionID, p.ID); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case loadedMsg:
		m.sessions = msg.sessions
		m.projects = msg.projects
		m.now = msg.now
		m.running = nil
		for i := range m.sessions {
			if m.sessions[i].Running() {
				s := m.sessions[i]
				m.running = &s
			}
		}
		return m, nil
	case eventsReadyMsg:
		m.events = msg.ch
		return m, waitForEvent(msg.ch)
	case statsLoadedMsg:
		m.today = msg.today
		m.burndown = msg.burndown
		return m, nil
	case eventMsg:
		return m, tea.Batch(m.reload(), m.reloadDayOffs(), m.reloadStats(), waitForEvent(m.events))
	case dayoffsLoadedMsg:
		m.dayoffs = msg.list
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.booking {
		return m.handleBookingKey(k)
	}
	switch {
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return m, tea.Quit
	case k.Text == "s":
		if m.running != nil || m.client == nil {
			return m, nil
		}
		m.status = "starting…"
		return m, m.startCmd()
	case k.Text == "x":
		if m.running == nil {
			return m, nil
		}
		m.booking = true
		m.sel = 0
		m.newName = ""
		return m, nil
	case k.Text == "d":
		m.showDayOffs = true
		return m, m.reloadDayOffs()
	case k.Code == tea.KeyEsc && m.showDayOffs:
		m.showDayOffs = false
		return m, nil
	}
	return m, nil
}

func (m Model) handleBookingKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.booking = false
		return m, nil
	case k.Code == tea.KeyEnter:
		id := m.running.ID
		m.booking = false
		if strings.TrimSpace(m.newName) != "" {
			return m, m.createAndStopCmd(id, strings.TrimSpace(m.newName))
		}
		if len(m.projects) == 0 {
			m.booking = true
			return m, nil
		}
		return m, m.stopCmd(id, m.projects[m.sel].ID)
	case k.Code == tea.KeyBackspace:
		if r := []rune(m.newName); len(r) > 0 {
			m.newName = string(r[:len(r)-1])
		}
		return m, nil
	case k.Text == "j" && m.newName == "":
		if m.sel < len(m.projects)-1 {
			m.sel++
		}
		return m, nil
	case k.Text == "k" && m.newName == "":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case k.Text != "":
		m.newName += k.Text
		return m, nil
	}
	return m, nil
}

func (m Model) View() tea.View {
	if m.showDayOffs {
		return m.dayOffView()
	}
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · worktime") + styleMuted.Render("  "+m.user) + "\n\n")

	if m.running != nil {
		el := m.running.Elapsed(m.now)
		b.WriteString(styleRunning.Render("▶ "+fmtDur(el)) + styleMuted.Render("  running") + "\n")
	} else {
		b.WriteString(styleMuted.Render("○ idle — press s to start") + "\n")
	}

	{
		logged := fmtMin(m.today.LoggedMin)
		target := fmtMin(m.today.TargetMin)
		saldo := m.today.SaldoMin
		line := fmt.Sprintf("  heute %s / %s · %s", logged, target, fmtSaldo(saldo))
		if saldo >= 0 {
			b.WriteString(styleOk.Render(line) + "\n")
		} else {
			b.WriteString(styleWarn.Render(line) + "\n")
		}
		if m.burndown.TargetMin > 0 {
			bd := fmt.Sprintf("  Monat %s / %s · %s", fmtMin(m.burndown.TotalMin), fmtMin(m.burndown.TargetMin), fmtSaldo(m.burndown.SaldoMin))
			b.WriteString(styleMuted.Render(bd) + "\n")
		}
	}
	b.WriteString("\n")

	if m.booking {
		b.WriteString(styleHeader.Render("Book session to a project") + "\n")
		for i, p := range m.projects {
			line := "  " + glyphOr(p.Glyph) + " " + p.Name
			if i == m.sel && m.newName == "" {
				line = styleSel.Render("▸ " + glyphOr(p.Glyph) + " " + p.Name)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(styleMuted.Render("  new: ") + m.newName + "▏\n")
		b.WriteString(styleMuted.Render("  j/k pick · type a name to create · enter confirm · esc cancel") + "\n")
	} else {
		b.WriteString(styleHeader.Render("Today") + "\n")
		if len(m.sessions) == 0 {
			b.WriteString(styleMuted.Render("  no sessions yet") + "\n")
		}
		for _, s := range m.sessions {
			mark := "·"
			if s.Running() {
				mark = "▶"
			}
			fmt.Fprintf(&b, "  %s %s  %s\n", mark, s.Start.Local().Format("15:04"), fmtDur(s.Elapsed(m.now)))
		}
	}

	b.WriteString("\n")
	if m.err != nil {
		b.WriteString(styleErr.Render("error: "+m.err.Error()) + "\n")
	}
	b.WriteString(styleMuted.Render("s start · x stop · q quit") + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func glyphOr(g string) string {
	if g == "" {
		return "●"
	}
	return g
}

func fmtMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

func fmtSaldo(min int) string {
	sign := "+"
	if min < 0 {
		sign = "-"
		min = -min
	}
	return fmt.Sprintf("%s%dh %02dm", sign, min/60, min%60)
}
