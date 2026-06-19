// Package week is the Worktime "Woche" sibling route: a read-only pace strip of
// the current week's days with colored progress bars.
package week

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// API is the narrow client surface WeekRoute needs (*apiclient.Client satisfies it).
type API interface {
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
}

type loadedMsg struct {
	days []apiclient.WeekDay
	err  error
}

// Route renders the current week. It reloads on session.* SSE events.
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	days   []apiclient.WeekDay
	loaded bool
	err    error
}

// NewRoute builds the Woche route. reg drives lateral w/t/d/e navigation.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg}
}

func (r *Route) Title() string { return "Woche" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		days, err := api.GetWeek(ctx, "")
		return loadedMsg{days: days, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err, r.days = true, m.err, m.days
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if cmd := navKey(r.reg, m); cmd != nil {
			return r, cmd
		}
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Woche lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	cells := 20
	var b strings.Builder
	b.WriteString("\n")
	for _, d := range r.days {
		marker := "  "
		if d.IsToday {
			marker = theme.Active(glyphs.Active, f.Pal) + " "
		}
		pct := 0
		if d.TargetMin > 0 {
			pct = d.LoggedMin * 100 / d.TargetMin
		}
		line := fmt.Sprintf("%s%s  %s  %s / %s",
			marker, d.Date, statusbar.Bar(pct, cells, f.Pal),
			wtfmt.FormatMin(d.LoggedMin), wtfmt.FormatMin(d.TargetMin))
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "t", Desc: "Stats"},
		{Key: "d", Desc: "Frei"},
		{Key: "e", Desc: "Export"},
		{Key: "esc", Desc: "zurück"},
	}
}

// navKey maps the sibling-switch keys through the registry. Returns nil for any
// other key. Shared shape across all sibling routes.
func navKey(reg wtnav.Registry, k tea.KeyPressMsg) tea.Cmd {
	switch k.Text {
	case "w", "t", "d", "e":
		return reg.Nav(k.Text)
	}
	return nil
}

// isSessionEvent reports whether the SSE event type is a worktime session event
// that should trigger a data reload.
func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}
