package writepicker_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
)

func TestInit_NilCmd(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init should return nil cmd, got %v", cmd)
	}
}

func TestSetPalette_NoPanic(t *testing.T) {
	t.Parallel()
	writepicker.SetPalette(theme.Default)
	writepicker.SetPalette(theme.Load())
}
