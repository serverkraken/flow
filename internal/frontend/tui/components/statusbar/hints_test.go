package statusbar_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestHints_ContainsText(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	out := statusbar.Hints("q → quit  ·  ? → help", p)
	if !strings.Contains(out, "q → quit") {
		t.Errorf("Hints output missing text: %q", out)
	}
}

func TestHints_EmptyString(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	out := statusbar.Hints("", p)
	if out == "" {
		t.Error("expected non-empty output (padding) even for empty text")
	}
}
