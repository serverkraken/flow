package confirm_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/ui/confirm"
)

func TestInit_ReturnsNil(t *testing.T) {
	t.Parallel()
	m := confirm.New("question?", "detail", testPalette)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init should return nil cmd, got %v", cmd)
	}
}
