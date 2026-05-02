package form_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

func TestNewTextInput_SetsPlaceholder(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	ti := form.NewTextInput("HH:MM", p)
	if ti.Placeholder != "HH:MM" {
		t.Errorf("placeholder = %q, want %q", ti.Placeholder, "HH:MM")
	}
}

func TestNewTextInput_SetsCharLimit(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	ti := form.NewTextInput("test", p)
	if ti.CharLimit != 80 {
		t.Errorf("charLimit = %d, want 80", ti.CharLimit)
	}
}
