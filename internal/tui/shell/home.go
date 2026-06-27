package shell

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// DashboardAPI is the narrow client surface the Home dashboard needs.
// *apiclient.Client satisfies it.
type DashboardAPI interface {
	GetToday(ctx context.Context) (apiclient.Today, error)
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
	ListDocuments(ctx context.Context, tags ...string) ([]domain.Document, error)
	ListNodes(ctx context.Context) ([]domain.Node, error)
}

type homeLoadedMsg struct {
	today    apiclient.Today
	week     []apiclient.WeekDay
	docs     []domain.Document
	projects []domain.Node
	err      error
}

// HomeRoute is the live dashboard: left column "Arbeit" (running session +
// Tagesziel bar + week pace), right column "Wissen" (recent docs + projects).
// Read-only; w/d drill into the Worktime/Docs tabs. Reloads on SSE events.
type HomeRoute struct {
	api      DashboardAPI
	pal      theme.Palette
	user     string
	today    apiclient.Today
	week     []apiclient.WeekDay
	docs     []domain.Document
	projects []domain.Node
	loaded   bool
	err      error
}

// NewHomeRoute builds the dashboard. api may be nil only in tests that never load.
func NewHomeRoute(api DashboardAPI, pal theme.Palette, user string) HomeRoute {
	return HomeRoute{api: api, pal: pal, user: user}
}

func (h HomeRoute) Title() string { return "Home" }

func (h HomeRoute) Init() tea.Cmd { return h.loadCmd() }

func (h HomeRoute) loadCmd() tea.Cmd {
	api := h.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := api.GetToday(ctx)
		if err != nil {
			return homeLoadedMsg{err: err}
		}
		week, _ := api.GetWeek(ctx, "")
		docs, _ := api.ListDocuments(ctx)
		projects, _ := api.ListNodes(ctx)
		return homeLoadedMsg{today: today, week: week, docs: docs, projects: projects}
	}
}

func (h HomeRoute) Update(msg tea.Msg) (Route, tea.Cmd) {
	switch m := msg.(type) {
	case homeLoadedMsg:
		h.loaded, h.err = true, m.err
		if m.err == nil {
			h.today, h.week, h.docs, h.projects = m.today, m.week, m.docs, m.projects
		}
		return h, nil
	case EventMsg:
		if homeRelevantEvent(m.Ev.Type) {
			return h, h.loadCmd()
		}
		return h, nil
	case tea.KeyPressMsg:
		switch m.Text {
		case "w":
			return h, func() tea.Msg { return SwitchTabMsg{Title: "Worktime"} }
		case "d":
			return h, func() tea.Msg { return SwitchTabMsg{Title: "Docs"} }
		}
	}
	return h, nil
}

func (h HomeRoute) View(f Frame) string {
	if !h.loaded {
		return theme.Dim("  Dashboard lädt …", f.Pal)
	}
	if h.err != nil {
		return theme.Dim("  Fehler: "+h.err.Error(), f.Pal)
	}
	left := h.workColumn(f.Pal)
	right := h.knowledgeColumn(f.Pal)
	if f.Width >= 80 {
		colW := f.Width/2 - 2
		l := lipgloss.NewStyle().Width(colW).Render(left)
		r := lipgloss.NewStyle().Width(colW).Render(right)
		return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, "  ", l, "  ", r)
	}
	return "\n" + left + "\n\n" + right
}

func (h HomeRoute) workColumn(pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Heading("Arbeit", pal) + "\n\n")
	state := theme.Dim("○ gestoppt", pal)
	if h.today.Running {
		state = theme.Active("● läuft", pal)
	}
	b.WriteString("  " + state + "\n")
	fmt.Fprintf(&b, "  Heute: %s / %s\n", fmtMin(h.today.LoggedMin), fmtMin(h.today.TargetMin))
	pct := 0
	if h.today.TargetMin > 0 {
		pct = h.today.LoggedMin * 100 / h.today.TargetMin
	}
	b.WriteString("  " + statusbar.Bar(pct, 16, pal) + "\n\n")
	b.WriteString(theme.Dim("  Woche", pal) + "\n")
	for _, d := range h.week {
		marker := "  "
		if d.IsToday {
			marker = theme.Active("▶ ", pal)
		}
		fmt.Fprintf(&b, "  %s%s  %s / %s\n", marker, d.Date, fmtMin(d.LoggedMin), fmtMin(d.TargetMin))
	}
	return b.String()
}

func (h HomeRoute) knowledgeColumn(pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Heading("Wissen", pal) + "\n\n")
	b.WriteString(theme.Dim("  Zuletzt bearbeitet", pal) + "\n")
	recent := append([]domain.Document(nil), h.docs...)
	sort.Slice(recent, func(i, j int) bool { return recent[i].UpdatedAt.After(recent[j].UpdatedAt) })
	if len(recent) > 5 {
		recent = recent[:5]
	}
	if len(recent) == 0 {
		b.WriteString(theme.Dim("  keine Dokumente", pal) + "\n")
	}
	for _, d := range recent {
		title := d.Title
		if title == "" {
			title = d.Path
		}
		b.WriteString("  • " + title + theme.Dim("  ("+d.Path+")", pal) + "\n")
	}
	b.WriteString("\n" + theme.Dim("  Projekte", pal) + "\n")
	if len(h.projects) == 0 {
		b.WriteString(theme.Dim("  keine Projekte", pal) + "\n")
	}
	for _, p := range h.projects {
		b.WriteString("  • " + p.Name + "\n")
	}
	return b.String()
}

func (h HomeRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "w", Desc: "Worktime"},
		{Key: "d", Desc: "Docs"},
		{Key: "tab", Desc: "Tab"},
		{Key: ":", Desc: "Palette"},
		{Key: "?", Desc: "Hilfe"},
	}
}

// fmtMin renders a non-negative minute count as "Xh YYm" (local to avoid
// coupling the shell to a worktime sub-package).
func fmtMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

func homeRelevantEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted,
		domain.EventDayOffChanged, domain.EventSettingsChanged,
		domain.EventNodeCreated:
		return true
	}
	// Any document.* event also refreshes the knowledge column.
	return strings.HasPrefix(t, "document.")
}
