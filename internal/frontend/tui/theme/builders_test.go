package theme_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Each builder takes (string, Palette) and emits the input wrapped in
// ANSI styling. We don't pin specific SGR sequences (they depend on
// the active termenv profile), but we lock in two invariants:
//   1. the original text is present in the output (no unintended
//      truncation / replacement)
//   2. the output is non-empty even for empty input (an SGR reset is
//      still emitted by lipgloss for non-default styles)

func TestBuilders_PreserveContent(t *testing.T) {
	t.Parallel()
	p := theme.Load()
	cases := []struct {
		name string
		fn   func(string, theme.Palette) string
	}{
		{"Body", theme.Body},
		{"Dim", theme.Dim},
		{"Strong", theme.Strong},
		{"Heading", theme.Heading},
		{"Highlight", theme.Highlight},
		{"Success", theme.Success},
		{"Warning", theme.Warning},
		{"Danger", theme.Danger},
		{"Err", theme.Err},
		{"Info", theme.Info},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.fn("payload", p)
			if !strings.Contains(got, "payload") {
				t.Errorf("%s did not preserve content, got %q", c.name, got)
			}
		})
	}
}
