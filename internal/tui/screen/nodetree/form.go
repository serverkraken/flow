package nodetree

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

// CreateFields / UpdateFields alias the apiclient field structs so this
// package and its tests reference them without importing apiclient directly.
type (
	CreateFields = apiclient.CreateNodeFields
	UpdateFields = apiclient.UpdateNodeFields
)

// FormAPI is the write surface the form needs. *apiclient.Client satisfies it.
type FormAPI interface {
	ListNodes(ctx context.Context) ([]domain.Node, error)
	CreateNode(ctx context.Context, in CreateFields) (domain.Node, error)
	UpdateNode(ctx context.Context, id string, in UpdateFields) (domain.Node, error)
	SetNodeRate(ctx context.Context, id string, amount *int64, currency string) error
}

var (
	_ FormAPI            = (*apiclient.Client)(nil)
	_ shell.TextCapturer = (*FormRoute)(nil)
)

// formErrMsg carries an API error back to the Update loop from an async cmd.
type formErrMsg struct{ err string }

// nodesLoadedMsg is returned by the Init cmd when ListNodes completes.
type nodesLoadedMsg struct {
	nodes []domain.Node
	err   error
}

// FormValues is the exported snapshot of form fields (test seam).
type FormValues struct {
	Name, Slug, Description, UpstreamGit string
	Kind, ParentID, Status, Color, Glyph string
	RateAmount, RateCurrency             string
}

// Focus order constants. Kind+Parent only appear in create mode;
// Rate only for engagement kind.
const (
	focusName int = iota
	focusSlug
	focusDescription
	focusUpstream
	focusKind
	focusParent
	focusStatus
	focusColor
	focusGlyph
	focusRateAmount
	focusRateCurrency
	focusCount
)

// textInputIdx maps focus index → inputs slice index.
// Selector-only fields are absent (they use cycle, not textinput).
var textInputIdx = map[int]int{
	focusName: 0, focusSlug: 1, focusDescription: 2, focusUpstream: 3,
	focusRateAmount: 4, focusRateCurrency: 5,
}

var (
	kindChoices   = []string{string(domain.KindEngagement), string(domain.KindVorhaben), string(domain.KindRepo)}
	statusChoices = []string{"active", "paused", "archived"}
	colorChoices  = append([]string{""}, domain.NodeColors...)
	glyphChoices  = append([]string{""}, domain.NodeGlyphs...)
)

// FormRoute is the node create/edit form. Owns all keys while active.
type FormRoute struct {
	api     FormAPI
	pal     theme.Palette
	editing *domain.Node // nil = create

	inputs   []textinput.Model // Name, Slug, Desc, Upstream, RateAmount, RateCurrency
	focusIdx int
	kindIx   int
	statusIx int
	colorIx  int
	glyphIx  int

	allNodes    []domain.Node
	parentIDs   []string // [""] + valid parent ids for current kind
	parentLabel []string
	parentIx    int

	err string
}

// NewFormRoute creates a new create form (editing==nil) or edit form (editing!=nil).
func NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Node) *FormRoute {
	r := &FormRoute{api: api, pal: pal, editing: editing}
	ph := []string{"Name", "slug", "Beschreibung", "https://…", "0.00", "EUR"}
	r.inputs = make([]textinput.Model, len(ph))
	for i, p := range ph {
		r.inputs[i] = form.NewTextInput(p, pal)
	}
	_ = r.inputs[0].Focus()
	r.statusIx = indexOf(statusChoices, "active")
	if editing != nil {
		r.inputs[0].SetValue(editing.Name)
		r.inputs[1].SetValue(editing.Slug)
		r.inputs[2].SetValue(editing.Description)
		r.inputs[3].SetValue(editing.UpstreamGit)
		r.kindIx = indexOf(kindChoices, string(editing.Kind))
		r.statusIx = indexOf(statusChoices, string(editing.Status))
		r.colorIx = indexOf(colorChoices, editing.Color)
		r.glyphIx = indexOf(glyphChoices, editing.Glyph)
		if editing.Rate != nil {
			r.inputs[4].SetValue(fmt.Sprintf("%d.%02d", editing.Rate.Amount/100, editing.Rate.Amount%100))
			r.inputs[5].SetValue(editing.Rate.Currency)
		}
	} else {
		r.inputs[5].SetValue("EUR")
	}
	r.recomputeParents()
	return r
}

// currentKind returns the node kind for the current form state.
// In edit mode, the kind is always the editing node's kind (immutable).
func (r *FormRoute) currentKind() domain.NodeKind {
	if r.editing != nil {
		return r.editing.Kind
	}
	return domain.NodeKind(kindChoices[r.kindIx])
}

// recomputeParents rebuilds the parent selector from allNodes for the current
// kind. Engagements are always roots — only "(Wurzel / keine)" is offered.
func (r *FormRoute) recomputeParents() {
	r.parentIDs = []string{""}
	r.parentLabel = []string{"(Wurzel / keine)"}
	if r.currentKind() == domain.KindEngagement {
		r.parentIx = 0
		return
	}
	kind := r.currentKind()
	for _, n := range r.allNodes {
		if domain.ValidParentKind(kind, n.Kind) {
			r.parentIDs = append(r.parentIDs, n.ID)
			r.parentLabel = append(r.parentLabel, n.Name+" ("+string(n.Kind)+")")
		}
	}
	if r.parentIx >= len(r.parentIDs) {
		r.parentIx = 0
	}
}

// CapturesInput implements shell.InputCapturer — form owns all keys.
func (r *FormRoute) CapturesInput() bool { return true }

// CapturesText implements shell.TextCapturer. The whole form is a literal
// text-entry surface, so the shell must forward every key — including q/Esc —
// to the form rather than treating them as global back-keys.
func (r *FormRoute) CapturesText() bool { return true }

// Title implements shell.Route.
func (r *FormRoute) Title() string {
	if r.editing != nil {
		return "Knoten bearbeiten"
	}
	return "Neuer Knoten"
}

// Init implements shell.Route — loads the node list for the parent selector.
func (r *FormRoute) Init() tea.Cmd {
	api := r.api
	loadNodes := func() tea.Msg {
		ns, err := api.ListNodes(context.Background())
		return nodesLoadedMsg{nodes: ns, err: err}
	}
	return tea.Batch(loadNodes, textinput.Blink)
}

// Values extracts the current form state (test seam + Submit internals).
func (r *FormRoute) Values() FormValues {
	pid := ""
	if r.parentIx >= 0 && r.parentIx < len(r.parentIDs) {
		pid = r.parentIDs[r.parentIx]
	}
	return FormValues{
		Name:         r.inputs[0].Value(),
		Slug:         r.inputs[1].Value(),
		Description:  r.inputs[2].Value(),
		UpstreamGit:  r.inputs[3].Value(),
		Kind:         kindChoices[r.kindIx],
		ParentID:     pid,
		Status:       statusChoices[r.statusIx],
		Color:        colorChoices[r.colorIx],
		Glyph:        glyphChoices[r.glyphIx],
		RateAmount:   r.inputs[4].Value(),
		RateCurrency: r.inputs[5].Value(),
	}
}

// FillForTest sets form state from a FormValues without driving key events.
// Used as a test seam.
func (r *FormRoute) FillForTest(v FormValues) {
	r.inputs[0].SetValue(v.Name)
	r.inputs[1].SetValue(v.Slug)
	r.inputs[2].SetValue(v.Description)
	r.inputs[3].SetValue(v.UpstreamGit)
	r.inputs[4].SetValue(v.RateAmount)
	r.inputs[5].SetValue(v.RateCurrency)
	if v.Kind != "" {
		r.kindIx = indexOf(kindChoices, v.Kind)
	}
	r.statusIx = indexOf(statusChoices, orDefault(v.Status, "active"))
	r.colorIx = indexOf(colorChoices, v.Color)
	r.glyphIx = indexOf(glyphChoices, v.Glyph)
	r.recomputeParents()
	r.parentIx = indexOf(r.parentIDs, v.ParentID)
}

// Submit validates synchronously then returns an async cmd.
// On validation error it sets r.err and returns (r, nil).
// On success the cmd calls the API and returns PopRouteMsg or formErrMsg.
func (r *FormRoute) Submit() (shell.Route, tea.Cmd) {
	v := r.Values()
	if strings.TrimSpace(v.Name) == "" {
		r.err = "Name erforderlich"
		return r, nil
	}
	kind := r.currentKind()
	// Parent is only required when creating a non-engagement node.
	// In edit mode, reparenting is done via the tree's move action (not this form).
	var parentPtr *string
	if r.editing == nil && kind != domain.KindEngagement {
		if strings.TrimSpace(v.ParentID) == "" {
			r.err = "Übergeordneter Knoten erforderlich"
			return r, nil
		}
		p := v.ParentID
		parentPtr = &p
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

	// Snapshot state for the async closure.
	api, editing := r.api, r.editing
	return r, func() tea.Msg {
		ctx := context.Background()
		var id string
		if editing != nil {
			id = editing.ID
			// Icon is deliberately omitted: nil = preserve whatever the server
			// already has (this form has no icon field to edit).
			if _, err := api.UpdateNode(ctx, id, UpdateFields{
				Name:        &v.Name,
				Slug:        &v.Slug,
				Color:       &v.Color,
				Glyph:       &v.Glyph,
				Description: &v.Description,
				UpstreamGit: &v.UpstreamGit,
				Status:      &v.Status,
			}); err != nil {
				return formErrMsg{fmt.Sprintf("Speichern: %v", err)}
			}
		} else {
			n, err := api.CreateNode(ctx, CreateFields{
				Name:        v.Name,
				Kind:        string(kind),
				ParentID:    parentPtr,
				Color:       v.Color,
				Glyph:       v.Glyph,
				Description: v.Description,
				UpstreamGit: v.UpstreamGit,
			})
			if err != nil {
				return formErrMsg{fmt.Sprintf("Anlegen: %v", err)}
			}
			id = n.ID
		}
		if kind == domain.KindEngagement {
			if err := api.SetNodeRate(ctx, id, rate, cur); err != nil {
				return formErrMsg{fmt.Sprintf("Satz: %v", err)}
			}
		}
		return shell.PopRouteMsg{}
	}
}

// Update implements shell.Route.
func (r *FormRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case nodesLoadedMsg:
		r.allNodes = m.nodes
		r.recomputeParents()
		return r, nil

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
			r.focusBy(-1)
			return r, nil

		case m.Code == tea.KeyTab || m.Code == tea.KeyDown:
			r.focusBy(1)
			return r, nil

		case m.Code == tea.KeyUp:
			r.focusBy(-1)
			return r, nil

		case m.Code == tea.KeyRight:
			r.cycle(1)
			return r, nil

		case m.Code == tea.KeyLeft:
			r.cycle(-1)
			return r, nil

		default:
			if ti, ok := textInputIdx[r.focusIdx]; ok {
				var cmd tea.Cmd
				r.inputs[ti], cmd = r.inputs[ti].Update(m)
				return r, cmd
			}
		}
	}
	return r, nil
}

// focusBy moves focus by delta steps, skipping inapplicable fields, and
// updating text-input focus state.
func (r *FormRoute) focusBy(d int) {
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		r.inputs[ti].Blur()
	}
	r.focusIdx = (r.focusIdx + d + focusCount) % focusCount
	for r.skip(r.focusIdx) {
		r.focusIdx = (r.focusIdx + sign(d) + focusCount) % focusCount
	}
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		_ = r.inputs[ti].Focus()
	}
}

// skip reports whether the given focus index should be skipped.
func (r *FormRoute) skip(idx int) bool {
	if r.editing != nil && (idx == focusKind || idx == focusParent) {
		return true
	}
	if r.currentKind() != domain.KindEngagement && (idx == focusRateAmount || idx == focusRateCurrency) {
		return true
	}
	return false
}

// cycle advances the selector at the current focus by delta.
func (r *FormRoute) cycle(d int) {
	switch r.focusIdx {
	case focusKind:
		if r.editing == nil {
			r.kindIx = (r.kindIx + d + len(kindChoices)) % len(kindChoices)
			r.recomputeParents()
		}
	case focusParent:
		if n := len(r.parentIDs); n > 0 {
			r.parentIx = (r.parentIx + d + n) % n
		}
	case focusStatus:
		r.statusIx = (r.statusIx + d + len(statusChoices)) % len(statusChoices)
	case focusColor:
		r.colorIx = (r.colorIx + d + len(colorChoices)) % len(colorChoices)
	case focusGlyph:
		r.glyphIx = (r.glyphIx + d + len(glyphChoices)) % len(glyphChoices)
	}
}

func sign(d int) int {
	if d < 0 {
		return -1
	}
	return 1
}

// parseRateCents converts a decimal amount string to minor units (cents).
// Blank → (nil, nil) which clears the rate. Negative/non-numeric → error.
func parseRateCents(amount string) (*int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	var whole, frac int64
	dotIdx := strings.Index(amount, ".")
	if dotIdx < 0 {
		n, err := parseUint64(amount)
		if err != nil {
			return nil, fmt.Errorf("ungültiger Satz %q", amount)
		}
		whole = n
	} else {
		wholePart := amount[:dotIdx]
		fracPart := amount[dotIdx+1:]
		if wholePart == "" {
			wholePart = "0"
		}
		n, err := parseUint64(wholePart)
		if err != nil {
			return nil, fmt.Errorf("ungültiger Satz %q", amount)
		}
		whole = n
		switch len(fracPart) {
		case 0:
			frac = 0
		case 1:
			f, err := parseUint64(fracPart)
			if err != nil {
				return nil, fmt.Errorf("ungültiger Satz %q", amount)
			}
			frac = f * 10
		default:
			f, err := parseUint64(fracPart[:2])
			if err != nil {
				return nil, fmt.Errorf("ungültiger Satz %q", amount)
			}
			frac = f
		}
	}
	c := whole*100 + frac
	return &c, nil
}

// parseUint64 parses a non-empty string of decimal digits into an int64.
func parseUint64(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty")
	}
	var n int64
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
