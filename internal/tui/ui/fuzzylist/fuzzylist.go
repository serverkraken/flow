// Package fuzzylist is a reusable, domain-free filterable list Model: type to
// fuzzy-filter, Up/Down (or Ctrl+n/Ctrl+p) to move, optional inline-create row.
// The caller owns Enter/Esc and the meaning of a selection.
package fuzzylist

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

// Item is one selectable entry. ID is opaque to the component.
type Item struct{ ID, Label string }

type entry struct {
	item Item
	idx  []int
}

// Model is a value type; Update/SetItems/etc. return a new copy.
type Model struct {
	items      []Item
	query      string
	filtered   []entry
	cursor     int
	pal        theme.Palette
	createHint string // e.g. "neu: %s"; empty disables the inline-create row
}

// New builds a list over items (kept in the given order; the caller supplies
// e.g. MRU order). The empty query shows them all.
func New(items []Item, pal theme.Palette) Model {
	return (Model{items: items, pal: pal}).refilter()
}

// WithCreateHint enables an inline-create row using hint as a printf format over
// the current query (e.g. "neu: %s").
func (m Model) WithCreateHint(hint string) Model { m.createHint = hint; return m.refilter() }

// SetItems replaces the items and re-filters, preserving the current query.
func (m Model) SetItems(items []Item) Model { m.items = items; return m.refilter() }

// Query is the current filter text.
func (m Model) Query() string { return m.query }

func (m Model) createActive() bool {
	if m.createHint == "" || strings.TrimSpace(m.query) == "" {
		return false
	}
	for _, it := range m.items {
		if strings.EqualFold(it.Label, m.query) {
			return false
		}
	}
	return true
}

func (m Model) rowCount() int {
	n := len(m.filtered)
	if m.createActive() {
		n++
	}
	return n
}

// Selection returns the entry under the cursor. isCreate is true when the cursor
// is on the inline-create row (the caller should create using Query()). ok is
// false when there is nothing selectable.
func (m Model) Selection() (it Item, isCreate, ok bool) {
	if m.createActive() && m.cursor == len(m.filtered) {
		return Item{}, true, true
	}
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor].item, false, true
	}
	return Item{}, false, false
}

// Update handles text→query, Backspace, and cursor movement (Up/Down +
// Ctrl+n/Ctrl+p). Every other rune (including j/k) is typed into the query.
func (m Model) Update(k tea.KeyPressMsg) Model {
	switch {
	case k.Code == tea.KeyUp || (k.Code == 'p' && k.Mod == tea.ModCtrl):
		if m.cursor > 0 {
			m.cursor--
		}
		return m
	case k.Code == tea.KeyDown || (k.Code == 'n' && k.Mod == tea.ModCtrl):
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		return m
	case k.Code == tea.KeyBackspace:
		if rn := []rune(m.query); len(rn) > 0 {
			m.query = string(rn[:len(rn)-1])
		}
		return m.refilter()
	case k.Text != "":
		m.query += k.Text
		return m.refilter()
	}
	return m
}

func (m Model) refilter() Model {
	out := make([]entry, 0, len(m.items))
	if m.query == "" {
		for _, it := range m.items {
			out = append(out, entry{item: it})
		}
	} else {
		type scored struct {
			e     entry
			score int
			ord   int
		}
		var ss []scored
		for i, it := range m.items {
			if idx, score, ok := fuzzymatch.Match(m.query, it.Label); ok {
				ss = append(ss, scored{e: entry{item: it, idx: idx}, score: score, ord: i})
			}
		}
		sort.SliceStable(ss, func(a, b int) bool {
			if ss[a].score != ss[b].score {
				return ss[a].score > ss[b].score
			}
			return ss[a].ord < ss[b].ord
		})
		for _, s := range ss {
			out = append(out, s.e)
		}
	}
	m.filtered = out
	if m.cursor >= m.rowCount() {
		m.cursor = m.rowCount() - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

// View renders the filtered rows (with match highlight) plus the inline-create
// row when active. width is the available content width.
func (m Model) View(width int) string {
	var b strings.Builder
	for i, e := range m.filtered {
		b.WriteString(picker.RowWithMatch(picker.RowWithMatchOpts{
			Selected: i == m.cursor,
			Label:    e.item.Label,
			Width:    width,
			Match:    e.idx,
		}, m.pal) + "\n")
	}
	if m.createActive() {
		label := glyphs.Extra + " " + fmt.Sprintf(m.createHint, m.query)
		b.WriteString(picker.Row(m.cursor == len(m.filtered), label, "", width, m.pal) + "\n")
	}
	return b.String()
}
