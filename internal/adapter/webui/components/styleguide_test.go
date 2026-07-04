package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestStyleguideShowcasesEverything(t *testing.T) {
	out := render(t, components.StyleguidePage())
	for _, w := range []string{
		"Design-System",              // styleguide.title
		"/static/app.css",            // Base hull present
		"/static/vendor/htmx.min.js", // offline
		"bg-red",                     // danger button variant
		"data-dialog-open",           // a dialog trigger
		"aria-modal=\"true\"",        // a dialog rendered
		"Abbrechen",                  // ConfirmDialog cancel
		"Seite 2",                    // Pagination middle page
		"Projekt",                    // a doc-kind badge
	} {
		if !strings.Contains(out, w) {
			t.Errorf("StyleguidePage missing %q", w)
		}
	}
}
