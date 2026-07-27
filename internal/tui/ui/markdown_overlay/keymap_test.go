package markdown_overlay

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

// containsKey reports whether binding b lists want among its keys.
func containsKey(b key.Binding, want string) bool {
	for _, k := range b.Keys() {
		if k == want {
			return true
		}
	}
	return false
}

func TestKeys_PageDown_IncludesCtrlD(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageDown, "ctrl+d") {
		t.Errorf("PageDown: expected ctrl+d alias for vim-style paging")
	}
}

func TestKeys_PageUp_IncludesCtrlU(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageUp, "ctrl+u") {
		t.Errorf("PageUp: expected ctrl+u alias for vim-style paging")
	}
}

func TestKeys_PageDown_IncludesPgDown(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageDown, "pgdown") {
		t.Errorf("PageDown: expected pgdown to remain available alongside vim alias")
	}
}

func TestKeys_PageUp_IncludesPgUp(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageUp, "pgup") {
		t.Errorf("PageUp: expected pgup to remain available alongside vim alias")
	}
}
