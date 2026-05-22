package worktime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// ansiFG returns the SGR escape-sequence body that lipgloss emits when
// rendering a theme.Color as a 24-bit foreground (i.e. "38;2;R;G;B"
// without the leading "\x1b[" or trailing "m"). The colorSeq lambdas
// in worktime tests use this to assert that a rendered cell carries
// the expected hue.
//
// Replaces termenv.RGBColor(hex).Sequence(false) from the v1 codebase
// — termenv is no longer a dependency under lipgloss v2 since the
// renderer abstraction it plugged into is gone.
func ansiFG(c theme.Color) string {
	s := strings.TrimPrefix(string(c), "#")
	if len(s) != 6 {
		return ""
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
}
