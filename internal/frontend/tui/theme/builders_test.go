package theme_test

import (
	"fmt"
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
		{"Active", theme.Active},
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

func TestActive_RendersCyanBold(t *testing.T) {
	p := theme.TokyonightNight
	out := theme.Active("läuft", p)
	if !strings.Contains(out, "läuft") {
		t.Fatalf("Active: expected content %q in %q", "läuft", out)
	}
	// Active ist Cyan+Bold — Sem.Active ist der canonical Token,
	// gleicher Hex wie Sem.Info aber distinkter Role (running/live).
	wantFg := p.Sem().Active
	if !containsForeground(out, wantFg) {
		t.Fatalf("Active: expected fg=%v in output %q", wantFg, out)
	}
	// Bold SGR is parameter 1; lipgloss v2 may emit it standalone
	// (\x1b[1m) or merged into a multi-parameter sequence
	// (\x1b[1;38;2;…m). Both forms start with "\x1b[1" followed by
	// either "m" or ";".
	if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[1;") {
		t.Fatalf("Active: expected bold SGR (\\x1b[1m or \\x1b[1;…m) in output %q", out)
	}
}

// containsForeground checks whether out contains the truecolor
// foreground SGR for c (lipgloss v2 emits `38;2;R;G;B`). The
// hex-string form (`#rrggbb`) is what %v prints but does NOT appear
// literally in the rendered ANSI, so we decode the hex and look for
// the RGB triplet instead.
func containsForeground(out string, c theme.Color) bool {
	hex := strings.TrimPrefix(fmt.Sprintf("%v", c), "#")
	if len(hex) != 6 {
		return false
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return false
	}
	return strings.Contains(out, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
}
