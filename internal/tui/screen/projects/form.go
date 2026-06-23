package projects

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/form"
)

// FormAPI is the narrow write surface the form route needs. A fake implements
// it in tests; *apiclient.Client satisfies it in production (enforced by the
// compile assert below).
type FormAPI interface {
	CreateProject(ctx context.Context, name string) (domain.Project, error)
	UpdateProject(ctx context.Context, id string, in UpdateFields) (domain.Project, error)
	SetProjectRate(ctx context.Context, projectID string, amount *int64, currency string) error
}

var _ FormAPI = (*apiclient.Client)(nil)

// formErrMsg carries an API error back to the Update loop from an async cmd.
type formErrMsg struct{ err string }

// FormValues is the exported snapshot of the form field values — used by
// FillForTest and Values so tests can inspect/set form state without driving key events.
type FormValues struct {
	Name, Slug, Description, UpstreamGit string
	Status, Color, Glyph                 string
	RateAmount, RateCurrency             string
}

// Focus order: 0=Name, 1=Slug, 2=Description, 3=UpstreamGit,
// 4=Status(selector), 5=Farbe(selector), 6=Glyph(selector),
// 7=RateAmount, 8=RateCurrency.
const (
	focusName int = iota
	focusSlug
	focusDescription
	focusUpstream
	focusStatus
	focusColor
	focusGlyph
	focusRateAmount
	focusRateCurrency
	focusCount
)

// textInputIdx maps a focus index to the inputs slice index.
// Returns -1 for selector fields.
var textInputIdx = map[int]int{
	focusName:         0,
	focusSlug:         1,
	focusDescription:  2,
	focusUpstream:     3,
	focusRateAmount:   4,
	focusRateCurrency: 5,
}

// colorChoices, glyphChoices, statusChoices are the selector whitelists.
// "" prepended to color/glyph signals "none".
var (
	colorChoices  = append([]string{""}, domain.ProjectColors...)
	glyphChoices  = append([]string{""}, domain.ProjectGlyphs...)
	statusChoices = []string{"active", "paused", "archived"}
)

// FormRoute is the project create/edit form. It owns keyboard input while
// active (CapturesInput → true).
type FormRoute struct {
	api     FormAPI
	pal     theme.Palette
	editing *domain.Project // nil = create mode

	// inputs holds the 6 text input widgets in focus order:
	// Name, Slug, Description, UpstreamGit, RateAmount, RateCurrency.
	inputs   []textinput.Model
	focusIdx int // overall focus (0–8)
	statusIx int
	colorIx  int
	glyphIx  int
	err      string
}

// NewFormRoute creates an empty form (create mode when editing==nil)
// or a pre-filled form (edit mode when editing!=nil).
func NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Project) *FormRoute {
	r := &FormRoute{api: api, pal: pal, editing: editing}

	// Build 6 themed text inputs.
	placeholders := []string{"Name", "slug", "Beschreibung", "https://github.com/…", "0.00", "EUR"}
	r.inputs = make([]textinput.Model, len(placeholders))
	for i, ph := range placeholders {
		r.inputs[i] = form.NewTextInput(ph, pal)
	}

	// Focus the first input.
	_ = r.inputs[0].Focus()

	// Pre-fill from editing project.
	if editing != nil {
		r.inputs[0].SetValue(editing.Name)
		r.inputs[1].SetValue(editing.Slug)
		r.inputs[2].SetValue(editing.Description)
		r.inputs[3].SetValue(editing.UpstreamGit)
		r.statusIx = indexOf(statusChoices, string(editing.Status))
		r.colorIx = indexOf(colorChoices, editing.Color)
		r.glyphIx = indexOf(glyphChoices, editing.Glyph)
		if editing.Rate != nil {
			r.inputs[4].SetValue(fmt.Sprintf("%d.%02d", editing.Rate.Amount/100, editing.Rate.Amount%100))
			r.inputs[5].SetValue(editing.Rate.Currency)
		}
	} else {
		// Default currency for new projects.
		r.inputs[5].SetValue("EUR")
		// Default status to active.
		r.statusIx = indexOf(statusChoices, "active")
	}

	return r
}

// CapturesInput implements shell.InputCapturer — form owns all keys.
func (r *FormRoute) CapturesInput() bool { return true }

// FocusIdx exposes the current focus index (test seam).
func (r *FormRoute) FocusIdx() int { return r.focusIdx }

// Values extracts the current field state (test seam + Submit internals).
func (r *FormRoute) Values() FormValues {
	return FormValues{
		Name:         r.inputs[0].Value(),
		Slug:         r.inputs[1].Value(),
		Description:  r.inputs[2].Value(),
		UpstreamGit:  r.inputs[3].Value(),
		Status:       statusChoices[r.statusIx],
		Color:        colorChoices[r.colorIx],
		Glyph:        glyphChoices[r.glyphIx],
		RateAmount:   r.inputs[4].Value(),
		RateCurrency: r.inputs[5].Value(),
	}
}

// FillForTest sets form state from a FormValues (test seam — no key events needed).
func (r *FormRoute) FillForTest(v FormValues) {
	r.inputs[0].SetValue(v.Name)
	r.inputs[1].SetValue(v.Slug)
	r.inputs[2].SetValue(v.Description)
	r.inputs[3].SetValue(v.UpstreamGit)
	r.inputs[4].SetValue(v.RateAmount)
	r.inputs[5].SetValue(v.RateCurrency)
	r.statusIx = indexOf(statusChoices, orDefault(v.Status, "active"))
	r.colorIx = indexOf(colorChoices, v.Color)
	r.glyphIx = indexOf(glyphChoices, v.Glyph)
}

// Submit validates fields synchronously (no I/O) then returns a tea.Cmd that
// executes the API compose off the event loop. On validation error it sets
// r.err and returns (r, nil). On success the cmd either returns
// shell.PopRouteMsg{} or formErrMsg{} which Update handles.
func (r *FormRoute) Submit() (shell.Route, tea.Cmd) {
	v := r.Values()
	if strings.TrimSpace(v.Name) == "" {
		r.err = "Name erforderlich"
		return r, nil
	}
	rate, perr := parseRateCents(v.RateAmount)
	if perr != nil {
		r.err = perr.Error()
		return r, nil
	}
	cur := strings.TrimSpace(v.RateCurrency)
	if cur == "" {
		cur = "EUR"
	}

	// Snapshot immutable state for the closure — r.editing pointer is safe
	// because the form is not re-created during the in-flight request.
	api := r.api
	editing := r.editing

	return r, func() tea.Msg {
		ctx := context.Background()
		var id string
		if editing != nil {
			id = editing.ID
		} else {
			p, err := api.CreateProject(ctx, v.Name)
			if err != nil {
				return formErrMsg{fmt.Sprintf("Projekt anlegen: %v", err)}
			}
			id = p.ID
		}
		if _, err := api.UpdateProject(ctx, id, UpdateFields{
			Name:        v.Name,
			Slug:        v.Slug,
			Color:       v.Color,
			Glyph:       v.Glyph,
			Description: v.Description,
			UpstreamGit: v.UpstreamGit,
			Status:      v.Status,
		}); err != nil {
			return formErrMsg{fmt.Sprintf("Projekt speichern: %v", err)}
		}
		// nil clears the rate; a value sets it.
		if err := api.SetProjectRate(ctx, id, rate, cur); err != nil {
			return formErrMsg{fmt.Sprintf("Satz setzen: %v", err)}
		}
		return shell.PopRouteMsg{}
	}
}

// Title implements shell.Route.
func (r *FormRoute) Title() string {
	if r.editing != nil {
		return "Projekt bearbeiten"
	}
	return "Neues Projekt"
}

// Init implements shell.Route.
func (r *FormRoute) Init() tea.Cmd { return textinput.Blink }

// Update implements shell.Route.
func (r *FormRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case formErrMsg:
		r.err = m.err
		return r, nil

	case tea.KeyPressMsg:
		switch {
		case m.Code == tea.KeyEsc:
			return r, func() tea.Msg { return shell.PopRouteMsg{} }

		case m.Code == tea.KeyEnter:
			return r.Submit()

		case m.Code == tea.KeyTab && m.Mod.Contains(tea.ModShift):
			r.focusNext(-1)
			return r, nil

		case m.Code == tea.KeyTab || m.Code == tea.KeyDown:
			r.focusNext(1)
			return r, nil

		case m.Code == tea.KeyUp:
			r.focusNext(-1)
			return r, nil

		case m.Code == tea.KeyRight:
			r.cycleSelector(1)
			return r, nil

		case m.Code == tea.KeyLeft:
			r.cycleSelector(-1)
			return r, nil

		default:
			// Forward to the focused text input.
			if ti, ok := textInputIdx[r.focusIdx]; ok {
				var cmd tea.Cmd
				r.inputs[ti], cmd = r.inputs[ti].Update(m)
				return r, cmd
			}
		}
	}
	return r, nil
}

// focusNext moves focus by delta steps (mod focusCount), updating text-input
// focus state.
func (r *FormRoute) focusNext(delta int) {
	// Blur current text input (if any).
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		r.inputs[ti].Blur()
	}
	r.focusIdx = (r.focusIdx + delta + focusCount) % focusCount
	// Focus new text input (if any).
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		_ = r.inputs[ti].Focus()
	}
}

// cycleSelector cycles the selector at the current focus position by delta.
// If the current focus is not a selector, this is a no-op.
func (r *FormRoute) cycleSelector(delta int) {
	switch r.focusIdx {
	case focusStatus:
		r.statusIx = (r.statusIx + delta + len(statusChoices)) % len(statusChoices)
	case focusColor:
		r.colorIx = (r.colorIx + delta + len(colorChoices)) % len(colorChoices)
	case focusGlyph:
		r.glyphIx = (r.glyphIx + delta + len(glyphChoices)) % len(glyphChoices)
	}
}

// parseRateCents converts a decimal amount string to cents (int64).
// Blank input → (nil, nil) which clears the rate.
// Negative or non-numeric input → error.
func parseRateCents(amount string) (*int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	var whole, frac int64
	// Try to parse as "X" or "X.Y" or "X.YY".
	dotIdx := strings.Index(amount, ".")
	if dotIdx < 0 {
		// Integer — parse as whole euros.
		n, err := parseInt64(amount)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("ungültiger Satz %q", amount)
		}
		whole = n
	} else {
		wholePart := amount[:dotIdx]
		fracPart := amount[dotIdx+1:]
		if wholePart == "" {
			wholePart = "0"
		}
		n, err := parseInt64(wholePart)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("ungültiger Satz %q", amount)
		}
		whole = n
		// Normalise fractional part to exactly 2 digits.
		switch len(fracPart) {
		case 0:
			frac = 0
		case 1:
			f, err := parseInt64(fracPart)
			if err != nil {
				return nil, fmt.Errorf("ungültiger Satz %q", amount)
			}
			frac = f * 10
		default:
			f, err := parseInt64(fracPart[:2])
			if err != nil {
				return nil, fmt.Errorf("ungültiger Satz %q", amount)
			}
			frac = f
		}
	}
	c := whole*100 + frac
	return &c, nil
}

// parseInt64 parses a non-negative integer string.
func parseInt64(s string) (int64, error) {
	var n int64
	if len(s) == 0 {
		return 0, fmt.Errorf("empty")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("non-digit %q", ch)
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

// indexOf returns the index of v in list, or 0 if not found.
func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

// orDefault returns v if non-empty, otherwise def.
func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
