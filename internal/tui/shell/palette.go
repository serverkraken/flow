package shell

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

// PaletteEntry is one selectable command/route in the palette.
type PaletteEntry struct {
	Label  string
	Action func() tea.Msg // returns the msg the Shell dispatches on select
}

// PaletteSelectedMsg is emitted when the user confirms an entry.
type PaletteSelectedMsg struct{ Entry PaletteEntry }

// PaletteDismissedMsg is emitted when the user presses Esc.
type PaletteDismissedMsg struct{}

// Palette is the :-command-palette model. Value type: the Shell holds it by
// value and reassigns on Update.
type Palette struct {
	entries  []PaletteEntry
	query    string
	cursor   int
	filtered []PaletteEntry
}

// NewPalette builds a palette over entries (empty query shows all).
func NewPalette(entries []PaletteEntry) Palette {
	return Palette{entries: entries}.refilter()
}

// SetQuery sets the filter text and resets the cursor.
func (p Palette) SetQuery(q string) Palette {
	p.query = q
	p.cursor = 0
	return p.refilter()
}

// Reset clears the query — call before opening.
func (p Palette) Reset() Palette { return p.SetQuery("") }

// Filtered returns the current matches.
func (p Palette) Filtered() []PaletteEntry { return p.filtered }

// Cursor returns the selected index.
func (p Palette) Cursor() int { return p.cursor }

// Update handles palette keys. On Enter emits PaletteSelectedMsg; on Esc
// emits PaletteDismissedMsg.
func (p Palette) Update(msg tea.Msg) (Palette, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch {
	case k.Code == tea.KeyEsc:
		return p, func() tea.Msg { return PaletteDismissedMsg{} }
	case k.Code == tea.KeyEnter:
		if len(p.filtered) == 0 {
			return p, func() tea.Msg { return PaletteDismissedMsg{} }
		}
		entry := p.filtered[p.cursor]
		return p, func() tea.Msg { return PaletteSelectedMsg{Entry: entry} }
	case k.Code == tea.KeyDown:
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
	case k.Code == tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case k.Code == tea.KeyBackspace:
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
			p = p.refilter()
		}
	case k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
		// printable (incl. Shift for capitals/symbols); ignore Ctrl/Alt combos
		p.query += k.Text
		p = p.refilter()
	}
	return p, nil
}

// View renders the palette inner content (query line + filtered rows). The
// Shell wraps this in a titlebox + overlay.
func (p Palette) View(width int, pal theme.Palette) string {
	var b strings.Builder
	b.WriteString(theme.Active(":", pal) + " " + theme.Body(p.query+"_", pal) + "\n")
	if len(p.filtered) == 0 {
		b.WriteString(theme.Dim("  keine Treffer", pal))
		return b.String()
	}
	for i, e := range p.filtered {
		b.WriteString(picker.Row(i == p.cursor, e.Label, "", width, pal))
		if i < len(p.filtered)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (p Palette) refilter() Palette {
	if p.query == "" {
		p.filtered = p.entries
	} else {
		q := strings.ToLower(p.query)
		out := make([]PaletteEntry, 0, len(p.entries))
		for _, e := range p.entries {
			if strings.Contains(strings.ToLower(e.Label), q) {
				out = append(out, e)
			}
		}
		p.filtered = out
	}
	if p.cursor >= len(p.filtered) {
		if len(p.filtered) > 0 {
			p.cursor = len(p.filtered) - 1
		} else {
			p.cursor = 0
		}
	}
	return p
}
