package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestMarkdownProseConstrainedForGridColumns(t *testing.T) {
	out := render(t, components.MarkdownProse(`<p>body</p>`))
	for _, want := range []string{`class="prose`, "min-w-0", "w-full", "max-w-[70ch]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("MarkdownProse missing width guard %q in %s", want, out)
		}
	}
}
