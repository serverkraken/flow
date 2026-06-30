package components

import (
	"bytes"
	"context"
	"testing"

	"github.com/a-h/templ"
)

// renderInternal renders a templ.Component to bytes.Buffer (not *runtime.Buffer)
// so the !IsBuffer defer block inside each generated template function executes.
// This covers the 2-statement defer block that production composition always skips.
func renderInternal(t *testing.T, comp templ.Component) {
	t.Helper()
	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Errorf("Render: %v", err)
	}
}

// TestStyleguideIsBuffer_Coverage exercises unexported styleguide_templ.go functions
// by calling them directly from the internal (same-package) test.
func TestStyleguideIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	renderInternal(t, styleguideBody())
	renderInternal(t, styleguideSubnav())
	renderInternal(t, styleguideContent())
	renderInternal(t, sgButtons())
	renderInternal(t, sgBadgesChips())
	renderInternal(t, sgStatTiles())
	renderInternal(t, sgEmpty())
	renderInternal(t, sgDialogs())
	renderInternal(t, sgPagination())
	renderInternal(t, sgKristallIntro())
	renderInternal(t, sgTokenSwatches())
	renderInternal(t, sgSwatch("--color-blue"))
	renderInternal(t, sgKristallCards())
	renderInternal(t, sgBrandMark())
}
