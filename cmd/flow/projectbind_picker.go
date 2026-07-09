package main

import (
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// repoName extracts the repository name from an origin slug.
// e.g. "serverkraken/flow" → "flow", "github.com/acme/app" → "app".
func repoName(originSlug string) string {
	s := strings.TrimSuffix(originSlug, ".git")
	return path.Base(s)
}

// pickerResult holds the outcome of the interactive picker after it completes.
type pickerResult struct {
	item      fuzzylist.Item
	isCreate  bool
	ok        bool
	cancelled bool
}

// pickProjectProgram is a one-shot bubbletea model wrapping fuzzylist.Model.
// It implements tea.Model: Init/Update/View delegate to the inner fuzzylist.
// Enter resolves the selection and quits; Esc/Ctrl-C cancels.
type pickProjectProgram struct {
	list   fuzzylist.Model
	title  string
	result pickerResult
}

// newPickProjectProgram builds the program model seeded with items and an
// optional defaultName pre-filled into the query (the repo name).
func newPickProjectProgram(items []fuzzylist.Item, defaultName string, pal theme.Palette) *pickProjectProgram {
	list := fuzzylist.New(items, pal).WithCreateHint("neu: %s")
	// Pre-fill the query with defaultName so the create hint shows it immediately.
	if defaultName != "" {
		for _, ch := range defaultName {
			list = list.Update(tea.KeyPressMsg{Text: string(ch)})
		}
	}
	return &pickProjectProgram{list: list, title: "Projekt wählen oder neu anlegen"}
}

// newPickParentProgram is the pick-only variant (no inline-create row) used to
// choose the engagement/vorhaben a freshly-created repo hangs under.
func newPickParentProgram(items []fuzzylist.Item, pal theme.Palette) *pickProjectProgram {
	return &pickProjectProgram{
		list:  fuzzylist.New(items, pal), // no create hint → pick-only
		title: "Engagement/Vorhaben als Eltern wählen",
	}
}

// newPickBookableProgram is the pick-only variant used by `flow worktime stop`
// to book the running session onto a node (Enter=book+stop, Esc=cancel).
func newPickBookableProgram(items []fuzzylist.Item, pal theme.Palette) *pickProjectProgram {
	return &pickProjectProgram{
		list:  fuzzylist.New(items, pal), // no create hint → pick-only
		title: "Zeit buchen auf …",
	}
}

// Init satisfies tea.Model. No I/O commands needed at startup.
func (m *pickProjectProgram) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (m *pickProjectProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyEnter:
			it, isCreate, ok := m.list.Selection()
			if !ok {
				return m, nil
			}
			if isCreate {
				it = fuzzylist.Item{Label: strings.TrimSpace(m.list.Query())}
			}
			m.result = pickerResult{item: it, isCreate: isCreate, ok: true}
			return m, tea.Quit
		case tea.KeyEsc:
			m.result = pickerResult{cancelled: true}
			return m, tea.Quit
		default:
			if msg.Code == 'c' && msg.Mod == tea.ModCtrl {
				m.result = pickerResult{cancelled: true}
				return m, tea.Quit
			}
			m.list = m.list.Update(msg)
		}
	}
	return m, nil
}

// Selection returns the resolved pick after the program has run.
// ok is false when cancelled or before Enter was pressed.
func (m *pickProjectProgram) Selection() (item fuzzylist.Item, isCreate, ok bool) {
	if m.result.cancelled || !m.result.ok {
		return fuzzylist.Item{}, false, false
	}
	return m.result.item, m.result.isCreate, true
}

// View satisfies tea.Model.
func (m *pickProjectProgram) View() tea.View {
	return tea.NewView("\n  " + m.title + "\n" +
		"  (tippen → filtern · ↑/↓ · enter · esc)\n\n" +
		m.list.View(60))
}
