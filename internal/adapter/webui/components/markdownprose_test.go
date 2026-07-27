package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestMarkdownProseConstrainedForGridColumns(t *testing.T) {
	// Lesesaal L3: the width guard now lives entirely in the named `.prose`
	// CSS class (web/tailwind.css, Task 1: max-width 680px + min-width:0) —
	// no arbitrary Tailwind utilities on the element itself anymore.
	out := render(t, components.MarkdownProse(`<p>body</p>`))
	if !strings.Contains(out, `class="prose"`) {
		t.Fatalf("MarkdownProse missing class=%q in %s", "prose", out)
	}
	for _, gone := range []string{"min-w-0", "w-full", "max-w-[70ch]"} {
		if strings.Contains(out, gone) {
			t.Fatalf("MarkdownProse should not carry arbitrary utility %q anymore, got %s", gone, out)
		}
	}
}
