// Package export is the Worktime "Export" sibling route: choose a preset/range
// + format + path and write the server export to disk.
package export

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// API is the narrow client surface ExportRoute needs.
type API interface {
	Export(ctx context.Context, from, to, format, projectID string) ([]byte, error)
}

type doneMsg struct{ path string }
type errMsg struct{ err error }

// Route is the export form. now is injected for deterministic tests.
type Route struct {
	api API
	now func() time.Time
	pal theme.Palette
	reg wtnav.Registry

	preset     string
	from, to   string
	format     string
	path       string
	pathEdited bool
	focus      int // 0=Range 1=von 2=bis 3=Format 4=Pfad
	status     string
}

// NewRoute builds the Export route defaulting to the current month.
func NewRoute(api API, now func() time.Time, pal theme.Palette, reg wtnav.Registry) *Route {
	if now == nil {
		now = time.Now
	}
	from, to := presetRange("monat", now())
	return &Route{
		api: api, now: now, pal: pal, reg: reg,
		preset: "monat", format: "md", from: from, to: to,
		path: defaultPath(from, to, "md"),
	}
}

func (r *Route) Title() string { return "Export" }
func (r *Route) Init() tea.Cmd { return nil }

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case doneMsg:
		r.status = "✓ geschrieben: " + m.path
		return r, nil
	case errMsg:
		r.status = "Fehler: " + m.err.Error()
		return r, nil
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	// Sibling-nav keys (w/t/d/e) pass through only when not in a text field.
	if cmd := navKey(r.reg, r.focus, k); cmd != nil {
		return r, cmd
	}
	switch {
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		r.focus = (r.focus + 4) % 5
	case k.Code == tea.KeyTab:
		r.focus = (r.focus + 1) % 5
	case k.Code == tea.KeyEnter:
		return r, r.submit()
	case k.Code == tea.KeyLeft, k.Code == tea.KeyRight:
		dir := 1
		if k.Code == tea.KeyLeft {
			dir = -1
		}
		r.cycleField(dir)
	case k.Code == tea.KeyBackspace:
		r.editField(func(s string) string {
			if rn := []rune(s); len(rn) > 0 {
				return string(rn[:len(rn)-1])
			}
			return s
		})
	case k.Text != "":
		t := k.Text
		r.editField(func(s string) string { return s + t })
	}
	return r, nil
}

// navKey passes w/t/d/e through the registry, but only when focus is on a
// non-text field (Range=0 or Format=3). Returns nil for any other key.
func navKey(reg wtnav.Registry, focus int, k tea.KeyPressMsg) tea.Cmd {
	if focus != 0 && focus != 3 {
		return nil
	}
	switch k.Text {
	case "w", "t", "d", "e":
		return reg.Nav(k.Text)
	}
	return nil
}

func (r *Route) cycleField(dir int) {
	switch r.focus {
	case 0:
		r.preset = cyclePreset(r.preset, dir)
		if r.preset != "custom" {
			r.from, r.to = presetRange(r.preset, r.now())
		}
		r.refreshPath()
	case 3:
		r.format = cycleFormat(r.format, dir)
		r.refreshPath()
	}
}

func (r *Route) editField(fn func(string) string) {
	switch r.focus {
	case 1:
		r.from = fn(r.from)
		r.preset = "custom"
		r.refreshPath()
	case 2:
		r.to = fn(r.to)
		r.preset = "custom"
		r.refreshPath()
	case 4:
		r.path = fn(r.path)
		r.pathEdited = true
	}
}

func (r *Route) refreshPath() {
	if !r.pathEdited {
		r.path = defaultPath(r.from, r.to, r.format)
	}
}

func (r *Route) submit() tea.Cmd {
	from, errF := time.Parse(dayFmt, r.from)
	to, errT := time.Parse(dayFmt, r.to)
	if errF != nil || errT != nil {
		r.status = "Ungültiges Datum (yyyy-mm-dd)"
		return nil
	}
	if to.Before(from) {
		r.status = "bis muss >= von sein"
		return nil
	}
	r.status = "exportiere…"
	api, fromS, toS, format, path := r.api, r.from, r.to, r.format, expandHome(r.path)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := api.Export(ctx, fromS, toS, format, "")
		if err != nil {
			return errMsg{err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return errMsg{err}
		}
		return doneMsg{path: path}
	}
}

func (r *Route) View(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n")
	field := func(idx int, label, val string) {
		cur := "  "
		if r.focus == idx {
			cur = theme.Active("▸", f.Pal) + " "
			val = theme.Active(val, f.Pal)
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cur, label, val)
	}
	field(0, "Range", r.preset)
	field(1, "von", r.from)
	field(2, "bis", r.to)
	field(3, "Format", r.format)
	field(4, "Pfad", r.path)
	if r.status != "" {
		b.WriteString("\n  " + theme.Dim(r.status, f.Pal) + "\n")
	}
	return b.String()
}

// KeyHints returns the key hints for the export form.
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "tab", Desc: "Feld"},
		{Key: "←/→", Desc: "wählen"},
		{Key: "enter", Desc: "export"},
		{Key: "esc", Desc: "zurück"},
	}
}

// WithPathForTest sets the path field directly (test seam for file-write tests).
func WithPathForTest(r *Route, path string) *Route {
	r.path = path
	r.pathEdited = true
	r.focus = 4
	return r
}

// WithDatesForTest overrides the from/to date strings directly (test seam for
// submit error-path tests — invalid date and to-before-from).
func WithDatesForTest(r *Route, from, to string) *Route {
	r.from = from
	r.to = to
	r.preset = "custom"
	return r
}
