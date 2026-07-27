package grammar

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKey_Matches(t *testing.T) {
	if !Special(tea.KeyDown).Matches(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatal("Special(KeyDown) must match a KeyDown press")
	}
	if Special(tea.KeyDown).Matches(tea.KeyPressMsg{Code: tea.KeyUp}) {
		t.Fatal("Special(KeyDown) must not match KeyUp")
	}
	if !Rune("q").Matches(tea.KeyPressMsg{Text: "q"}) {
		t.Fatal("Rune(q) must match a 'q' text press")
	}
	if Rune("q").Matches(tea.KeyPressMsg{Text: "q", Mod: tea.ModCtrl}) {
		t.Fatal("Rune(q) must not match Ctrl+q")
	}
	if !Ctrl('c').Matches(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}) {
		t.Fatal("Ctrl(c) must match Ctrl+C")
	}
}

func TestBinding_Matches_Back(t *testing.T) {
	if !Back.Matches(tea.KeyPressMsg{Text: "q"}) {
		t.Fatal("Back must match q")
	}
	if !Back.Matches(tea.KeyPressMsg{Code: tea.KeyEsc}) {
		t.Fatal("Back must match Esc")
	}
}

func TestBinding_Hint(t *testing.T) {
	h := MoveDown.Hint()
	if h.Key != "↑/↓" || h.Desc != "bewegen" {
		t.Fatalf("MoveDown.Hint() = %+v, want {↑/↓ bewegen}", h)
	}
}

func TestNoVimKeysAdvertised(t *testing.T) {
	all := []Binding{MoveUp, MoveDown, Top, Bottom, PageUp, PageDown, Open, Back, Quit, Search, Help, NextTab}
	for _, b := range all {
		for _, bad := range []string{"j", "k", "g", "G"} {
			if b.KeyLabel == bad {
				t.Fatalf("binding %s advertises vim key %q", b.ID, bad)
			}
		}
	}
}
