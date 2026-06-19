// Package dayoffs is the Worktime "Frei" sibling route: day-offs/holidays list
// plus default-target editing, add/delete day-off, and Bundesland selection.
package dayoffs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface DayOffsRoute needs (*apiclient.Client satisfies it).
type API interface {
	ListDayOffs(ctx context.Context, from, to string) ([]apiclient.DayOff, error)
	GetSettings(ctx context.Context) (apiclient.Settings, error)
	SetTargetConfig(ctx context.Context, defaultMin int, weekday map[string]int) error
	AddDayOffs(ctx context.Context, from, to, kind, label string, targetMin int, skipWeekends bool) error
	DeleteDayOff(ctx context.Context, day string) error
	SetBundesland(ctx context.Context, land string) error
}

type loadedMsg struct {
	list     []apiclient.DayOff
	settings apiclient.Settings
	err      error
}

type reloadMsg struct{}

// Route renders the day-offs/holidays list for the current year.
// It reloads on dayoff.changed + settings.changed SSE events.
type Route struct {
	api      API
	pal      theme.Palette
	reg      wtnav.Registry
	now      func() time.Time
	list     []apiclient.DayOff
	settings apiclient.Settings
	cursor   int
	loaded   bool
	err      error

	dialog dialogKind
	dlg    dialogState
}

// NewRoute builds the Frei route. reg drives lateral w/t/d/e navigation.
// now is the clock function; pass nil to use time.Now.
func NewRoute(api API, pal theme.Palette, reg wtnav.Registry, now func() time.Time) *Route {
	if now == nil {
		now = time.Now
	}
	return &Route{api: api, pal: pal, reg: reg, now: now}
}

func (r *Route) Title() string { return "Frei" }

// CapturesInput reports that the route owns the keyboard while a dialog is open.
// Implements shell.InputCapturer.
func (r *Route) CapturesInput() bool { return r.dialog != dialogNone }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		year := time.Now().Year()
		list, err := api.ListDayOffs(ctx, fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
		if err != nil {
			return loadedMsg{err: err}
		}
		st, err := api.GetSettings(ctx)
		return loadedMsg{list: list, settings: st, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded, r.err = true, m.err
		if m.err == nil {
			r.list, r.settings = m.list, m.settings
			if r.cursor >= len(r.list) {
				if len(r.list) > 0 {
					r.cursor = len(r.list) - 1
				} else {
					r.cursor = 0
				}
			}
		}
		return r, nil
	case reloadMsg:
		return r, r.loadCmd()
	case confirm.ResultMsg:
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.cursor < len(r.list) {
			return r, r.deleteCmd(r.list[r.cursor].Day)
		}
		return r, nil
	case shell.EventMsg:
		if isDayoffEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k)
	}
	switch k.Text {
	case "j":
		if len(r.list) > 0 {
			r.cursor = (r.cursor + 1) % len(r.list)
		}
	case "k":
		if len(r.list) > 0 {
			r.cursor = (r.cursor + len(r.list) - 1) % len(r.list)
		}
	case "g":
		return r.openTargetEdit()
	case "a":
		return r.openAdd()
	case "D":
		return r.openDelete()
	case "b":
		return r.openBundesland()
	case "w", "t", "d", "e":
		return r, r.reg.Nav(k.Text)
	}
	return r, nil
}

func (r *Route) View(f shell.Frame) string {
	if !r.loaded {
		return theme.Dim("  Frei lädt …", f.Pal)
	}
	if r.err != nil {
		return theme.Dim("  Fehler: "+r.err.Error(), f.Pal)
	}
	if r.dialog != dialogNone {
		return r.renderDialog(f)
	}
	var b strings.Builder
	b.WriteString("\n")
	target := fmt.Sprintf("  Tagesziel: %s", wtfmt.FormatMin(r.settings.DefaultTargetMin))
	if len(r.settings.WeekdayTargetMin) > 0 {
		type kv struct{ k, v string }
		var ov []kv
		for day, mins := range r.settings.WeekdayTargetMin {
			ov = append(ov, kv{weekdayShort(day), wtfmt.FormatMin(mins)})
		}
		sort.Slice(ov, func(i, j int) bool { return ov[i].k < ov[j].k })
		parts := make([]string, 0, len(ov))
		for _, o := range ov {
			parts = append(parts, o.k+" "+o.v)
		}
		target += theme.Dim("  ("+strings.Join(parts, ", ")+")", f.Pal)
	}
	land := r.settings.Bundesland
	if land == "" {
		land = "—"
	}
	b.WriteString(target + "\n")
	b.WriteString(theme.Dim("  Bundesland: "+land, f.Pal) + "\n\n")
	if len(r.list) == 0 {
		b.WriteString(theme.Dim("  keine Frei-Tage dieses Jahr", f.Pal) + "\n")
	}
	for i, d := range r.list {
		label := d.Label
		if label == "" {
			label = d.Kind
		}
		row := fmt.Sprintf("  %s %s  %s", dayOffGlyph(d.Holiday, f.Pal), d.Day, label)
		if i == r.cursor {
			row = theme.Active(row, f.Pal)
		}
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (r *Route) KeyHints() []keyhint.Hint {
	if r.dialog != dialogNone {
		return r.dialogHints()
	}
	return []keyhint.Hint{
		{Key: "g/a/D", Desc: "Ziel/Add/Del"},
		{Key: "b", Desc: "Bundesland"},
		{Key: "w/t/e", Desc: "Woche/Stats/Export"},
		{Key: "esc", Desc: "zurück"},
	}
}

func dayOffGlyph(holiday bool, pal theme.Palette) string {
	if holiday {
		return theme.Dim("○", pal)
	}
	return "○"
}

func weekdayShort(key string) string {
	names := map[string]string{
		"0": "So", "1": "Mo", "2": "Di",
		"3": "Mi", "4": "Do", "5": "Fr", "6": "Sa",
	}
	if n, ok := names[key]; ok {
		return n
	}
	return key
}

func isDayoffEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventDayOffChanged, domain.EventSettingsChanged:
		return true
	}
	return false
}
