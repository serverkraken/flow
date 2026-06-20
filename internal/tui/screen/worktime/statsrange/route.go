// Package statsrange is the Worktime "Stats" sibling route: aggregate stats for
// the current week or month plus a burndown line. No heatmap (deferred to M3d).
package statsrange

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
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface StatsRangeRoute needs.
type API interface {
	GetStats(ctx context.Context, rng string) (apiclient.Stats, error)
	GetBurndown(ctx context.Context) (apiclient.Burndown, error)
}

type loadedMsg struct {
	rng   string
	stats apiclient.Stats
	bd    apiclient.Burndown
	err   error
}

// Route renders stats for rng ("week"|"month"). m/W toggle the range.
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	rng    string
	stats  apiclient.Stats
	bd     apiclient.Burndown
	loaded bool
	err    error
}

// NewRoute builds the Stats route defaulting to the current week.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg, rng: "week"}
}

func (r *Route) Title() string { return "Stats" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api, rng := r.api, r.rng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := api.GetStats(ctx, rng)
		if err != nil {
			return loadedMsg{rng: rng, err: err}
		}
		bd, _ := api.GetBurndown(ctx)
		return loadedMsg{rng: rng, stats: st, bd: bd}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err = true, m.err
		if m.err == nil {
			r.rng, r.stats, r.bd = m.rng, m.stats, m.bd
		}
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if cmd := wtnav.Lateral(r.reg, wtnav.IdxStats, m); cmd != nil {
			return r, cmd
		}
		switch m.Text {
		case "m":
			if r.rng != "month" {
				r.rng = "month"
				return r, r.loadCmd()
			}
			return r, nil
		case "W":
			if r.rng != "week" {
				r.rng = "week"
				return r, r.loadCmd()
			}
			return r, nil
		default:
			if cmd := navKey(r.reg, m); cmd != nil {
				return r, cmd
			}
		}
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	strip := wtnav.Strip(wtnav.IdxStats, f.Width, f.Pal) + "\n"
	if !r.loaded {
		return strip + theme.Dim("  Stats lädt …", f.Pal)
	}
	if r.err != nil {
		return strip + theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	label := "KW"
	if r.rng == "month" {
		label = "Monat"
	}
	s := r.stats
	rows := [][2]string{
		{"Zeitraum", label},
		{"Total", wtfmt.FormatMin(s.TotalMin)},
		{"⌀/Tag", wtfmt.FormatMin(s.AvgMin)},
		{"Max", wtfmt.FormatMin(s.MaxMin)},
		{"Min", wtfmt.FormatMin(s.MinMin)},
		{"Arbeitstage", fmt.Sprintf("%d", s.Workdays)},
		{"Treffer", fmt.Sprintf("%d/%d", s.Hits, s.Workdays)},
		{"Streak", fmt.Sprintf("%d (best %d)", s.Streak, s.BestStreak)},
		{"Saldo", wtfmt.FormatSaldo(s.OvertimeMin)},
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-12s %s\n", row[0], row[1])
	}
	if r.bd.TargetMin > 0 {
		b.WriteString("\n  " + theme.Dim(fmt.Sprintf("Burndown: %s / %s · %s",
			wtfmt.FormatMin(r.bd.TotalMin), wtfmt.FormatMin(r.bd.TargetMin),
			wtfmt.FormatSaldo(r.bd.SaldoMin)), f.Pal) + "\n")
	}
	return strip + b.String()
}

// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "m/W", Desc: "Monat/KW"},
		{Key: "←/→", Desc: "Bereich"},
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
