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
	rows     []dayRow
	projects []domain.Project // fetched alongside sessions to resolve row project names
	err      error
}

// Route lists one day's completed (and running) sessions. It satisfies shell.Route.
type Route struct {
	api    API
	pal    theme.Palette
	day    time.Time // start-of-day (00:00:00 local time)
	rows   []dayRow
	cur    listnav.Cursor
	loaded bool
	err    error
	toast  toast.Model

	// nachb is non-nil while the Nachbuchen (Add) dialog is open.
	nachb    *nachbuchenState
	projects []domain.Project  // cached project list for the dialog
	projName map[string]string // id→name for resolving project names in rows
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
		// Fetch projects too so rows can render the project NAME, not the ID.
		// A project-list failure must not hide the sessions, so ignore its error.
		ps, _ := api.ListProjects(ctx)
		return loadedMsg{rows: buildRows(sessions, day), projects: ps}
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

// loadProjectsCmd fetches the project list for the Nachbuchen dialog.
func (r *Route) loadProjectsCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, err := api.ListProjects(ctx)
		return nachbuchenLoadProjectsMsg{projects: ps, err: err}
	}
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
			if m.projects != nil {
				r.projects = m.projects
				r.projName = make(map[string]string, len(m.projects))
				for _, p := range m.projects {
					r.projName[p.ID] = p.Name
				}
			}
		}
		return r, nil

	case nachbuchenLoadProjectsMsg:
		if m.err != nil {
			r.toast = toast.NewDanger("Projekte konnten nicht geladen werden: "+m.err.Error(), r.pal)
			return r, r.toast.Init()
		}
		r.projects = m.projects
		r.nachb = openNachbuchen(r.pal, r.projects)
		return r, nil

	case nachbuchenProjectMsg:
		if m.err != nil {
			r.toast = toast.NewDanger("Projekt konnte nicht erstellt werden: "+m.err.Error(), r.pal)
			return r, r.toast.Init()
		}
		// Inline-create succeeded — advance to Von.
		if r.nachb != nil {
			id := m.id
			r.nachb.projID = &id
			r.nachb.projName = m.name
			r.nachb.focus = focusVon
			_ = r.nachb.von.Focus()
		}
		return r, nil

	case nachbuchenDoneMsg:
		if m.err != nil {
			r.toast = toast.NewDanger("Konnte nicht speichern: "+m.err.Error(), r.pal)
			return r, r.toast.Init()
		}
		r.toast = toast.NewSuccess("Session gespeichert", r.pal)
		return r, tea.Batch(r.toast.Init(), r.loadCmd())

	case shell.EventMsg:
		if isSessionEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil

	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil

	case tea.KeyPressMsg:
		// While the Nachbuchen dialog is open, forward all keys to it.
		if r.nachb != nil {
			return r.handleNachbuchenKey(m)
		}
		if c, ok := r.cur.Handle(m, len(r.rows), 5); ok {
			r.cur = c
			return r, nil
		}
		if grammar.Back.Matches(m) {
			return r, func() tea.Msg { return shell.PopRouteMsg{} }
		}
		if m.Text == "n" {
			// Open Nachbuchen: load projects first (or use cache).
			if len(r.projects) > 0 {
				r.nachb = openNachbuchen(r.pal, r.projects)
				return r, nil
			}
			return r, r.loadProjectsCmd()
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

	// While the Nachbuchen dialog is open, render it instead of the list.
	if r.nachb != nil {
		b.WriteString(r.renderNachbuchen(f))
		// Show toast below dialog if visible.
		for _, line := range toast.SlotRows(&r.toast, "  ") {
			b.WriteString(line)
			b.WriteString("\n")
		}
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
			label := renderRowLabel(row, r.projName)
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

// renderRowLabel formats the primary "Von–Bis · Projekt · Dauer" display of a
// session row. projName resolves a row's ProjectID to its display name; when the
// id is unknown (map miss) the raw id is shown, and an unset project is omitted.
func renderRowLabel(row dayRow, projName map[string]string) string {
	start := row.Start.Format("15:04")
	proj := resolveProjectName(row.Project, projName)
	if row.Running {
		if proj != "" {
			return fmt.Sprintf("%s → läuft   ·  %s", start, proj)
		}
		return fmt.Sprintf("%s → läuft", start)
	}
	stop := row.Stop.Format("15:04")
	durMin := int(row.Dur.Minutes())
	if proj != "" {
		return fmt.Sprintf("%s → %s  ·  %s  ·  %s", start, stop, proj, wtfmt.FormatMin(durMin))
	}
	return fmt.Sprintf("%s → %s   %s", start, stop, wtfmt.FormatMin(durMin))
}

// resolveProjectName maps a row's ProjectID to its display name. An empty id
// (no project) yields "". A miss in the map falls back to the raw id so a stale
// cache never hides which project a row belongs to.
func resolveProjectName(id string, projName map[string]string) string {
	if id == "" {
		return ""
	}
	if name, ok := projName[id]; ok {
		return name
	}
	return id
}

// renderRowHint returns the trailing dim hint for a row (the tag, if any).
func renderRowHint(row dayRow) string {
	if row.Tag != "" {
		return "[" + row.Tag + "]"
	}
	return ""
}

// KeyHints returns the advertised key bindings for the footer strip.
func (r *Route) KeyHints() []keyhint.Hint {
	if r.nachb != nil {
		return nachbuchenHints()
	}
	return []keyhint.Hint{
		grammar.MoveUp.Hint(),
		{Key: "n", Desc: "Nachbuchen"},
		{Key: "esc", Desc: "zurück"},
	}
}
