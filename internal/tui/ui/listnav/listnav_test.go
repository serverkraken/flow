package listnav

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }

func TestHandle_Movement(t *testing.T) {
	c := New() // idx 0
	c, ok := c.Handle(press(tea.KeyDown), 3, 2)
	if !ok || c.Index() != 1 {
		t.Fatalf("down: idx=%d ok=%v, want 1 true", c.Index(), ok)
	}
	c, _ = c.Handle(press(tea.KeyDown), 3, 2) // 2
	c, _ = c.Handle(press(tea.KeyDown), 3, 2) // clamp at 2
	if c.Index() != 2 {
		t.Fatalf("clamp bottom: idx=%d, want 2", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyUp), 3, 2)
	c, _ = c.Handle(press(tea.KeyUp), 3, 2)
	c, _ = c.Handle(press(tea.KeyUp), 3, 2) // clamp at 0
	if c.Index() != 0 {
		t.Fatalf("clamp top: idx=%d, want 0", c.Index())
	}
}

func TestHandle_HomeEndPage(t *testing.T) {
	c := New()
	c, _ = c.Handle(press(tea.KeyEnd), 10, 4)
	if c.Index() != 9 {
		t.Fatalf("End: idx=%d, want 9", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyHome), 10, 4)
	if c.Index() != 0 {
		t.Fatalf("Home: idx=%d, want 0", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyPgDown), 10, 4)
	if c.Index() != 4 {
		t.Fatalf("PgDown: idx=%d, want 4", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyPgUp), 10, 4)
	if c.Index() != 0 {
		t.Fatalf("PgUp clamps: idx=%d, want 0", c.Index())
	}
}

func TestHandle_NotANavKey(t *testing.T) {
	c := New()
	_, ok := c.Handle(tea.KeyPressMsg{Text: "n"}, 3, 2)
	if ok {
		t.Fatal("'n' is not a nav key; ok must be false")
	}
}

func TestHandle_EmptyList(t *testing.T) {
	c := New()
	c, _ = c.Handle(press(tea.KeyDown), 0, 2)
	if c.Index() != 0 {
		t.Fatalf("empty: idx=%d, want 0", c.Index())
	}
}
