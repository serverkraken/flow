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
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
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
	vonDP      datepicker.Model
	bisDP      datepicker.Model
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
	von := mustDate(from)
	bis := mustDate(to)
	r := &Route{
		api: api, now: now, pal: pal, reg: reg,
		preset: "monat", format: "md",
		vonDP: datepicker.New(von, pal), bisDP: datepicker.New(bis, pal),
		path: defaultPath(from, to, "md"),
	}
	r.vonDP.Focus()
	return r
}

// mustDate parses a yyyy-mm-dd produced by presetRange; presetRange always emits
// valid dates, so a parse failure is a programming error.
func mustDate(s string) time.Time {
	t, err := time.Parse(dayFmt, s)
	if err != nil {
		panic("export: bad preset date " + s)
	}
	return t
}

func (r *Route) Title() string { return "Export" }
func (r *Route) Init() tea.Cmd { return nil }

// CapturesInput reports that the export form owns the keyboard at every focus:
// it is a multi-field form (Range/von/bis/Format/Pfad), so Tab/Shift+Tab field
// cycling, ←/→ value+date editing, and digits must all reach the route rather
// than the shell. q/Esc still reach the back chain (handled in the shell before
// this guard), returning to Heute. Implements shell.InputCapturer.
func (r *Route) CapturesInput() bool { return true }

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
	if k.Code == tea.KeyEsc {
		return r, func() tea.Msg { return shell.PopRouteMsg{} }
	}
	// Sibling-nav keys (w/t/d/e) pass through only when not in a text field.
	if cmd := navKey(r.reg, r.focus, k); cmd != nil {
		return r, cmd
	}

	// Route keys to the focused date picker when von (1) or bis (2) is active.
	if r.focus == 1 || r.focus == 2 {
		if k.Text == "t" {
			r.setFocusedDate(r.now().Format(dayFmt))
			return r, nil
		}
		switch k.Code {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown:
			r.editFocusedPicker(k)
			r.preset = "custom"
			r.refreshPath()
			return r, nil
		case tea.KeyTab:
			// fall through to Tab handling below
		default:
			if len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9' {
				r.editFocusedPicker(k)
				r.preset = "custom"
				r.refreshPath()
				return r, nil
			}
		}
	}

	switch {
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		r.focus = (r.focus + 4) % 5
		r.syncPickerFocus()
	case k.Code == tea.KeyTab:
		r.focus = (r.focus + 1) % 5
		r.syncPickerFocus()
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
			f, to := presetRange(r.preset, r.now())
			_ = r.vonDP.SetValue(f)
			_ = r.bisDP.SetValue(to)
		}
		r.refreshPath()
	case 3:
		r.format = cycleFormat(r.format, dir)
		r.refreshPath()
	}
}

func (r *Route) editField(fn func(string) string) {
	switch r.focus {
	case 4:
		r.path = fn(r.path)
		r.pathEdited = true
	}
}

func (r *Route) editFocusedPicker(k tea.KeyPressMsg) {
	if r.focus == 1 {
		r.vonDP = r.vonDP.Update(k)
	} else {
		r.bisDP = r.bisDP.Update(k)
	}
}

func (r *Route) setFocusedDate(s string) {
	if r.focus == 1 {
		_ = r.vonDP.SetValue(s)
	} else {
		_ = r.bisDP.SetValue(s)
	}
	r.preset = "custom"
	r.refreshPath()
}

func (r *Route) syncPickerFocus() {
	r.vonDP.Blur()
	r.bisDP.Blur()
	switch r.focus {
	case 1:
		r.vonDP.Focus()
	case 2:
		r.bisDP.Focus()
	}
}

func (r *Route) refreshPath() {
	if !r.pathEdited {
		r.path = defaultPath(r.vonDP.Value(), r.bisDP.Value(), r.format)
	}
}

func (r *Route) submit() tea.Cmd {
	from := r.vonDP.Value()
	to := r.bisDP.Value()
	if to < from {
		r.status = "bis muss >= von sein"
		return nil
	}
	r.status = "exportiere…"
	api, format, path := r.api, r.format, expandHome(r.path)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := api.Export(ctx, from, to, format, "")
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
	fieldRaw := func(idx int, label, val string) {
		cur := "  "
		if r.focus == idx {
			cur = theme.Active("▸", f.Pal) + " "
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cur, label, val)
	}
	field(0, "Range", r.preset)
	fieldRaw(1, "von", r.vonDP.View())
	fieldRaw(2, "bis", r.bisDP.View())
	field(3, "Format", r.format)
	field(4, "Pfad", r.path)
	switch r.focus {
	case 1:
		b.WriteString("\n" + r.vonDP.Calendar(r.now()))
	case 2:
		b.WriteString("\n" + r.bisDP.Calendar(r.now()))
	}
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
	r.syncPickerFocus()
	return r
}

// WithDatesForTest overrides the von/bis date pickers directly (test seam).
// Invalid date strings are ignored (picker keeps its current value).
func WithDatesForTest(r *Route, from, to string) *Route {
	_ = r.vonDP.SetValue(from)
	_ = r.bisDP.SetValue(to)
	r.preset = "custom"
	return r
}
