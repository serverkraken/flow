package worktime

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/tui-kit/components/picker"
	"github.com/serverkraken/tui-kit/components/statusbar"
	"github.com/serverkraken/tui-kit/components/titlebox"
	tk "github.com/serverkraken/tui-kit/theme"
)

type tickMsg time.Time

type dayLoadedMsg struct {
	day Day
	err error
}

type actionDoneMsg struct{ err error }

// Model is the bubbletea model for the worktime screen.
type Model struct {
	day    Day
	now    time.Time
	err    error
	theme  tk.Palette
	width  int
	height int
}

// New creates a new worktime Model.
func New(p tk.Palette) Model {
	return Model{
		theme: p,
		now:   time.Now(),
	}
}

// FilterActive always returns false — worktime has no text filter.
func (m Model) FilterActive() bool { return false }

// StateFilter returns "" — worktime has no filter to persist.
func (m Model) StateFilter() string { return "" }

// StateCursor returns 0 — worktime has no cursor to persist.
func (m Model) StateCursor() int { return 0 }

// Init loads today's worktime data and starts the per-second tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), tickCmd())
}

// Update handles messages for the worktime screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tickCmd()

	case dayLoadedMsg:
		m.err = msg.err
		if msg.err == nil {
			m.day = msg.day
		}
		return m, nil

	case actionDoneMsg:
		// Reload after start/stop to pick up new state.
		return m, m.loadCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			return m, m.toggleCmd()
		}
	}
	return m, nil
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		day, err := LoadToday()
		return dayLoadedMsg{day: day, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) toggleCmd() tea.Cmd {
	running := m.day.IsRunning()
	return func() tea.Msg {
		home, _ := userHome()
		script := filepath.Join(home, ".tmux", "plugins", "worktime", "worktime.sh")
		arg := "start"
		if running {
			arg = "stop"
		}
		err := exec.Command(script, arg).Run()
		return actionDoneMsg{err: err}
	}
}

// View renders the worktime screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	inner := m.width - 4

	var rows []string
	rows = append(rows, "")

	if m.err != nil {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.Red).Render("  "+m.err.Error()))
	} else {
		rows = append(rows, m.renderDay(inner)...)
	}
	rows = append(rows, "")

	body := strings.Join(rows, "\n")

	status := "pausiert"
	if m.day.IsRunning() {
		status = "läuft ▶"
	}
	box := titlebox.Render("Worktime · "+status, body, m.width, m.theme)

	toggle := "s → starten"
	if m.day.IsRunning() {
		toggle = "s → stoppen"
	}
	footer := lipgloss.NewStyle().Foreground(m.theme.Dim).Padding(0, 1).
		Render(toggle + "  ·  q → schließen")
	return box + "\n" + footer
}

func (m Model) renderDay(inner int) []string {
	now := m.now
	total := m.day.Total(now)
	target := m.day.Target
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
		if pct > 100 {
			pct = 100
		}
	}

	kv := func(k, v string, vc lipgloss.Color) string {
		ks := lipgloss.NewStyle().Foreground(m.theme.Dim).Width(12).Render(k)
		vs := lipgloss.NewStyle().Foreground(vc).Bold(true).Render(v)
		return "  " + ks + vs
	}

	var valColor lipgloss.Color
	switch {
	case total >= target:
		valColor = m.theme.Red
	case total >= target*6/8:
		valColor = m.theme.Yellow
	default:
		valColor = m.theme.Green
	}

	rows := []string{
		kv("heute", formatDur(total), valColor),
		kv("ziel", formatDur(target), m.theme.Dim),
		kv("verbleibend", formatDur(remainingDur(total, target)), m.theme.Orange),
	}
	if m.day.Active != nil {
		rows = append(rows, "")
		rows = append(rows, kv("gestartet", m.day.Active.Format("15:04"), m.theme.Fg))
		eta := m.day.Active.Add(target - m.day.Logged)
		rows = append(rows, kv("eta", eta.Format("15:04"), m.theme.Fg))
	}
	rows = append(rows, "")
	barCells := inner - 8
	if barCells < 4 {
		barCells = 4
	}
	pctStr := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(fmt.Sprintf("%d%%", pct))
	rows = append(rows, "  "+statusbar.Bar(pct, barCells, m.theme)+"  "+pctStr)

	if len(m.day.Sessions) > 0 {
		rows = append(rows, "")
		rows = append(rows, picker.SectionHeader("Sitzungen heute", inner, m.theme))
		for _, s := range m.day.Sessions {
			label := fmt.Sprintf("%s → %s  %s",
				s.Start.Format("15:04"),
				s.Stop.Format("15:04"),
				formatDur(s.Elapsed),
			)
			rows = append(rows, "  "+lipgloss.NewStyle().Foreground(m.theme.Fg).Render(label))
		}
	}

	return rows
}

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %02dm", h, m)
}

func remainingDur(total, target time.Duration) time.Duration {
	r := target - total
	if r < 0 {
		return 0
	}
	return r
}

func userHome() (string, error) {
	home := ""
	out, err := exec.Command("sh", "-c", "echo $HOME").Output()
	if err == nil {
		home = strings.TrimSpace(string(out))
	}
	return home, err
}
