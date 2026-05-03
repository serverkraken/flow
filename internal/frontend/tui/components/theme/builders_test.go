package theme_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

func TestBuilders_PreserveText(t *testing.T) {
	t.Parallel()
	p := theme.Load()

	cases := map[string]func(string, theme.Palette) string{
		"Body":      theme.Body,
		"Dim":       theme.Dim,
		"Strong":    theme.Strong,
		"Heading":   theme.Heading,
		"Highlight": theme.Highlight,
		"Success":   theme.Success,
		"Warning":   theme.Warning,
		"Danger":    theme.Danger,
		"Err":       theme.Err,
		"Info":      theme.Info,
	}
	const input = "hello"
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			got := fn(input, p)
			if !strings.Contains(got, input) {
				t.Errorf("%s output %q does not contain input %q", name, got, input)
			}
		})
	}
}
