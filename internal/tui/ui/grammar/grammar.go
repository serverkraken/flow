// Package grammar is the single source of truth for flow's keyboard grammar.
// Each Binding pairs the keys that trigger an action with the canonical German
// hint advertised for it, so behaviour (Matches) and footer/help text (Hint)
// are defined once and cannot drift. The grammar is deliberately default-user
// oriented (arrows, Home/End, q/Esc back) — not vim (no j/k/g/G).
package grammar

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Key is one triggering key: a special key (arrows, Home…), a printable rune,
// or a Ctrl-modified key.
type Key struct {
	code rune
	text string
	mod  tea.KeyMod
}

// Special matches a non-printable key by its code (e.g. tea.KeyUp).
func Special(c rune) Key { return Key{code: c} }

// Rune matches a printable key by the text it produces (e.g. "q", "/").
func Rune(s string) Key { return Key{text: s} }

// Ctrl matches a Ctrl-modified key by code (e.g. Ctrl('c')).
func Ctrl(c rune) Key { return Key{code: c, mod: tea.ModCtrl} }

// Matches reports whether k triggers this key.
func (key Key) Matches(k tea.KeyPressMsg) bool {
	if key.text != "" {
		return k.Mod == 0 && k.Text == key.text
	}
	return k.Code == key.code && k.Mod == key.mod
}

// Binding is one grammar entry. KeyLabel/Desc are the advertised hint halves.
type Binding struct {
	ID       string
	Keys     []Key
	KeyLabel string
	Desc     string
}

// Matches reports whether k triggers any of this binding's keys.
func (b Binding) Matches(k tea.KeyPressMsg) bool {
	for _, key := range b.Keys {
		if key.Matches(k) {
			return true
		}
	}
	return false
}

// Hint renders this binding as a footer key-hint.
func (b Binding) Hint() keyhint.Hint { return keyhint.Hint{Key: b.KeyLabel, Desc: b.Desc} }

// Canonical structural bindings — the contract, defined once.
var (
	MoveUp   = Binding{ID: "move.up", Keys: []Key{Special(tea.KeyUp)}, KeyLabel: "↑/↓", Desc: "bewegen"}
	MoveDown = Binding{ID: "move.down", Keys: []Key{Special(tea.KeyDown)}, KeyLabel: "↑/↓", Desc: "bewegen"}
	Top      = Binding{ID: "jump.top", Keys: []Key{Special(tea.KeyHome)}, KeyLabel: "pos1/ende", Desc: "sprung"}
	Bottom   = Binding{ID: "jump.bottom", Keys: []Key{Special(tea.KeyEnd)}, KeyLabel: "pos1/ende", Desc: "sprung"}
	PageUp   = Binding{ID: "page.up", Keys: []Key{Special(tea.KeyPgUp)}, KeyLabel: "bild↑/↓", Desc: "blättern"}
	PageDown = Binding{ID: "page.down", Keys: []Key{Special(tea.KeyPgDown)}, KeyLabel: "bild↑/↓", Desc: "blättern"}
	Open     = Binding{ID: "open", Keys: []Key{Special(tea.KeyEnter)}, KeyLabel: "enter", Desc: "öffnen"}
	Back     = Binding{ID: "back", Keys: []Key{Rune("q"), Special(tea.KeyEsc)}, KeyLabel: "q", Desc: "zurück"}
	Quit     = Binding{ID: "quit", Keys: []Key{Rune("q"), Special(tea.KeyEsc)}, KeyLabel: "q", Desc: "beenden"}
	Search   = Binding{ID: "search", Keys: []Key{Rune("/")}, KeyLabel: "/", Desc: "suchen"}
	Help     = Binding{ID: "help", Keys: []Key{Rune("?")}, KeyLabel: "?", Desc: "Hilfe"}
	NextTab  = Binding{ID: "tab.next", Keys: []Key{Special(tea.KeyTab)}, KeyLabel: "tab", Desc: "wechseln"}
	WeekPrev = Binding{ID: "week.prev", Keys: []Key{Rune("[")}, KeyLabel: "[", Desc: "Woche zurück"}
	WeekNext = Binding{ID: "week.next", Keys: []Key{Rune("]")}, KeyLabel: "]", Desc: "Woche vor"}
)
