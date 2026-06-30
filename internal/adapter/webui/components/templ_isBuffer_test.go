package components_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// TestExportedComponentsIsBuffer exercises small exported template functions
// by calling them directly so their !IsBuffer defer blocks (2 stmts each) are covered.
// Using the shared render() helper (which writes to bytes.Buffer, not *runtime.Buffer)
// triggers IsBuffer=false in each function's GetBuffer call.

func TestActorGlyph_IsBuffer(t *testing.T) {
	render(t, components.ActorGlyph("engagement"))
}

func TestBrandMark_IsBuffer(t *testing.T) {
	render(t, components.BrandMark("h-6 w-6", "test"))
}

func TestBacklinks_IsBuffer(t *testing.T) {
	render(t, components.Backlinks(nil))
}

func TestCardCorner_IsBuffer(t *testing.T) {
	render(t, components.CardCorner("blue", "extra"))
}

func TestToc_IsBuffer(t *testing.T) {
	render(t, components.Toc("toc-main"))
}
