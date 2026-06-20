// Package daydetail is a leaf shell.Route that lists one day's worktime
// sessions. It is pushed from the Woche route when the user drills into a day.
// It must NOT import the worktime package to avoid a dependency cycle.
package daydetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

// loadedMsg carries the result of a ListSessionsRange fetch.
type loadedMsg struct {
	rows []dayRow
	err  error
}

// Route lists one day's completed (and running) sessions. It is a leaf — no
// dialogs; those arrive in Tasks 6/7. It satisfies shell.Route.
type Route struct {
	api    API
	pal    theme.Palette
	day    time.Time // start-of-day (00:00:00 local time)
	rows   []dayRow
	cur    listnav.Cursor
	loaded bool
	err    error
	toast  toast.Model
}

// NewRoute builds a DayDetail route for the given date. Only the date part of
// day is used — the time component is normalised to midnight in day's Location.
func NewRoute(api API, pal theme.Palette, day time.Time) *Route {
	y, m, d := day.Date()
	startOfDay := time.Date(y, m, d, 0, 0, 0, 0, day.Location())
	return &Route{
		api: api,
		pal: pal,
		day: startOfDay,
		cur: listnav.New(),
	}
}

// Title returns the route breadcrumb label.
func (r *Route) Title() string {
	return "Tag · " + r.day.Format("02.01.2006")
}

// Init triggers the first data load.
func (r *Route) Init() tea.Cmd {
	return r.loadCmd()
}

// loadCmd fetches sessions for exactly [startOfDay, startOfDay+24h).
func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	since := r.day
	until := r.day.Add(24 * time.Hour)
	day := r.day
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sessions, err := api.ListSessionsRange(ctx, since, until)
		if err != nil {
			return loadedMsg{err: err}
		}
		return loadedMsg{rows: buildRows(sessions, day)}
	}
}

// isSessionEvent reports whether the SSE event type is a worktime session event
// that should trigger a data reload. Copied here to avoid importing the worktime
// package (acyclic constraint).
func isSessionEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventSessionStarted, domain.EventSessionStopped,
		domain.EventSessionUpdated, domain.EventSessionDeleted:
		return true
	}
	return false
}

// Update handles all incoming messages.
func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.err = m.err
		if m.err == nil {
			r.rows = m.rows
			r.cur = r.cur.Clamp(len(r.rows))
		}
		return r, nil

	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil

	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil

	case tea.KeyPressMsg:
		if c, ok := r.cur.Handle(m, len(r.rows), 5); ok {
			r.cur = c
			return r, nil
		}
		if grammar.Back.Matches(m) {
			return r, func() tea.Msg { return shell.PopRouteMsg{} }
		}
	}
	return r, nil
}

// View renders the day's session list within the given frame.
func (r *Route) View(f shell.Frame) string {
	var b strings.Builder

	if !r.loaded {
		b.WriteString(theme.Dim("  Tag lädt …", f.Pal))
		return b.String()
	}
	if r.err != nil {
		b.WriteString(theme.Dim("  Fehler: "+r.err.Error(), f.Pal))
		return b.String()
	}

	inner := f.Width - 4
	if inner < 10 {
		inner = 10
	}

	if len(r.rows) == 0 {
		b.WriteString("\n")
		b.WriteString(theme.Dim("  Keine Buchungen — n zum Nachbuchen", f.Pal))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		for i, row := range r.rows {
			label := renderRowLabel(row)
			hint := renderRowHint(row)
			b.WriteString(picker.Row(i == r.cur.Index(), label, hint, inner, f.Pal))
			b.WriteString("\n")
		}
	}

	// Toast slot
	for _, line := range toast.SlotRows(&r.toast, "  ") {
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// renderRowLabel formats the primary "Von–Bis · Dauer" display of a session row.
func renderRowLabel(row dayRow) string {
	start := row.Start.Format("15:04")
	if row.Running {
		return fmt.Sprintf("%s → läuft", start)
	}
	stop := row.Stop.Format("15:04")
	durMin := int(row.Dur.Minutes())
	return fmt.Sprintf("%s → %s   %s", start, stop, wtfmt.FormatMin(durMin))
}

// renderRowHint returns the trailing dim hint for a row (tag only for Task 4;
// project name added in Task 6).
func renderRowHint(row dayRow) string {
	if row.Tag != "" {
		return "[" + row.Tag + "]"
	}
	return ""
}

// KeyHints returns the advertised key bindings for the footer strip.
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		grammar.MoveUp.Hint(),
		{Key: "esc", Desc: "zurück"},
	}
}
