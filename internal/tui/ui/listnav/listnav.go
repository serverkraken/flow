// Package listnav is the shared list-cursor primitive. It owns a clamped index
// and maps keyboard keys to movement using the canonical grammar bindings
// (arrows, Home/End, PageUp/PageDown). Clamp, never wrap; no j/k. It is a pure
// value — screens embed a Cursor and call Handle in their Update.
package listnav

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/grammar"
)

// Cursor is a clamped selection index.
type Cursor struct{ idx int }

// New returns a Cursor at index 0.
func New() Cursor { return Cursor{} }

// Index returns the current index.
func (c Cursor) Index() int { return c.idx }

// Clamp returns c with its index bounded to [0, count-1] (0 when count<=0).
func (c Cursor) Clamp(count int) Cursor {
	if count <= 0 {
		c.idx = 0
		return c
	}
	if c.idx < 0 {
		c.idx = 0
	}
	if c.idx > count-1 {
		c.idx = count - 1
	}
	return c
}

// Set returns c moved to i, clamped to count.
func (c Cursor) Set(i, count int) Cursor { c.idx = i; return c.Clamp(count) }

// Handle maps k to a clamped movement against count. ok is false when k is not a
// navigation key (the caller keeps handling it). pageSize is the PageUp/Down step.
func (c Cursor) Handle(k tea.KeyPressMsg, count, pageSize int) (Cursor, bool) {
	switch {
	case grammar.MoveDown.Matches(k):
		return c.Set(c.idx+1, count), true
	case grammar.MoveUp.Matches(k):
		return c.Set(c.idx-1, count), true
	case grammar.Top.Matches(k):
		return c.Set(0, count), true
	case grammar.Bottom.Matches(k):
		return c.Set(count-1, count), true
	case grammar.PageDown.Matches(k):
		return c.Set(c.idx+pageSize, count), true
	case grammar.PageUp.Matches(k):
		return c.Set(c.idx-pageSize, count), true
	}
	return c, false
}
