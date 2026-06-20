// Package week is the Worktime "Woche" sibling route: a read-only pace strip of
// the current week's days with colored progress bars.
package week

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
)

// API is the narrow client surface WeekRoute needs (*apiclient.Client satisfies it).
type API interface {
	GetWeek(ctx context.Context, ref string) ([]apiclient.WeekDay, error)
	ListDayOffs(ctx context.Context, from, to string) ([]apiclient.DayOff, error)
}

type loadedMsg struct {
	days []apiclient.WeekDay
	offs []apiclient.DayOff
	err  error
}

// Route renders the current week. It reloads on session.* SSE events.
type Route struct {
	api    API
	pal    theme.Palette
	reg    wtnav.Registry
	days   []apiclient.WeekDay
	offs   map[string]apiclient.DayOff
	cur    listnav.Cursor // selected day index (clamped, arrows/Home/End/PgUp/PgDn)
	offset int            // week offset from current week (0 = current, -1 = last week, etc.)
	loaded bool
	err    error
}

// NewRoute builds the Woche route. reg drives lateral w/t/d/e navigation.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry) *Route {
	return &Route{api: api, pal: pal, reg: reg, cur: listnav.New()}
}

// SelectedIndex returns the index of the currently selected day row.
// It is used by View and exposed for testing.
func (r *Route) SelectedIndex() int { return r.cur.Index() }

func (r *Route) Title() string { return "Woche" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

// weekRef returns a YYYY-MM-DD inside the week `offset` weeks from now (offset<=0).
// Returns "" when offset==0 to let the server resolve the current week.
func (r *Route) weekRef() string {
	if r.offset == 0 {
		return ""
	}
	return time.Now().AddDate(0, 0, r.offset*7).Format("2006-01-02")
}

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	ref := r.weekRef()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		days, err := api.GetWeek(ctx, ref)
		if err != nil {
			return loadedMsg{err: err}
		}
		var offs []apiclient.DayOff
		if len(days) > 0 {
			offs, err = api.ListDayOffs(ctx, days[0].Date, days[len(days)-1].Date)
		}
		return loadedMsg{days: days, offs: offs, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err, r.days = true, m.err, m.days
		r.offs = make(map[string]apiclient.DayOff, len(m.offs))
		for _, o := range m.offs {
			r.offs[o.Day] = o
		}
		r.cur = r.cur.Clamp(len(r.days))
		return r, nil
	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if c, ok := r.cur.Handle(m, len(r.days), 0); ok {
			r.cur = c
			return r, nil
		}
		switch {
		case grammar.WeekPrev.Matches(m):
			r.offset--
			return r, r.loadCmd()
		case grammar.WeekNext.Matches(m):
			if r.offset < 0 {
				r.offset++
				return r, r.loadCmd()
			}
			return r, nil // clamp: no future weeks
		}
		if cmd := wtnav.Lateral(r.reg, wtnav.IdxWoche, m); cmd != nil {
			return r, cmd
		}
		if cmd := navKey(r.reg, m); cmd != nil {
			return r, cmd
		}
	}
	return r, nil
}

// weekRangeHeader returns a header line showing the week range derived from r.days.
// Example: "‹ KW 25  2026-06-15 – 2026-06-21 ›"
func (r *Route) weekRangeHeader(pal theme.Palette) string {
	if len(r.days) == 0 {
		return ""
	}
	first := r.days[0].Date
	last := r.days[len(r.days)-1].Date
	// Parse first date to get ISO week number.
	t, err := time.Parse("2006-01-02", first)
	if err != nil {
		return "  ‹ " + first + " – " + last + " ›"
	}
	_, kw := t.ISOWeek()
	label := fmt.Sprintf("‹ KW %d  %s – %s ›", kw, first, last)
	return "  " + theme.Dim(label, pal)
}

func (r *Route) View(f shell.Frame) string {
	strip := wtnav.Strip(wtnav.IdxWoche, f.Width, f.Pal) + "\n"
	if !r.loaded {
		return strip + theme.Dim("  Woche lädt …", f.Pal)
	}
	if r.err != nil {
		return strip + theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if header := r.weekRangeHeader(f.Pal); header != "" {
		strip += header + "\n"
	}
	cells := 20
	sem := f.Pal.Sem()
	selBarStyle := lipgloss.NewStyle().Foreground(sem.Accent)
	selLabelStyle := lipgloss.NewStyle().Foreground(f.Pal.Fg).Bold(true)
	defLabelStyle := lipgloss.NewStyle().Foreground(f.Pal.Fg)
	var b strings.Builder
	b.WriteString("\n")
	for i, d := range r.days {
		selected := i == r.cur.Index()
		// Left edge: selection bar (▎) for cursor row, space otherwise.
		// IsToday marker (▶) is shown independently of the cursor.
		var selBar string
		if selected {
			selBar = selBarStyle.Render(glyphs.AccentBar)
		} else {
			selBar = " "
		}
		todayMarker := " "
		if d.IsToday {
			todayMarker = theme.Active(glyphs.Active, f.Pal)
		}
		var detail string
		if off, ok := r.offs[d.Date]; ok {
			label := off.Label
			if label == "" {
				label = off.Kind
			}
			detail = theme.Dim(label, f.Pal)
		} else if isWeekendDate(d.Date) {
			detail = theme.Dim("Wochenende", f.Pal)
		} else {
			pct := 0
			if d.TargetMin > 0 {
				pct = d.LoggedMin * 100 / d.TargetMin
			}
			detail = fmt.Sprintf("%s  %s / %s",
				statusbar.Bar(pct, cells, f.Pal),
				wtfmt.FormatMin(d.LoggedMin), wtfmt.FormatMin(d.TargetMin))
		}
		var dateStr string
		if selected {
			dateStr = selLabelStyle.Render(d.Date)
		} else {
			dateStr = defLabelStyle.Render(d.Date)
		}
		b.WriteString(" " + selBar + " " + todayMarker + " " + dateStr + "  " + detail + "\n")
	}
	b.WriteString(r.renderSummary(f.Width))
	return strip + b.String()
}

// HideBreadcrumb implements shell.BreadcrumbHider.
func (r *Route) HideBreadcrumb() bool { return true }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "←/→", Desc: "Bereich"},
		grammar.WeekPrev.Hint(),
		grammar.WeekNext.Hint(),
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
