package components_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

// testCtx is the bare context used by tests that call templ.Component.Render
// directly instead of going through the render helper (e.g. when a *strings.Builder
// is needed instead of *bytes.Buffer).
func testCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func TestBaseHullIsOfflineAndThemed(t *testing.T) {
	body := templ.Raw(`<p id="content">hallo</p>`)
	out := render(t, components.Base("Test", body))

	wants := []string{
		`<!doctype html>`,                       // templ lowercases the doctype
		`lang="de"`,
		`/static/app.css`,
		`/static/vendor/htmx.min.js`,            // local, NOT unpkg
		`/static/vendor/htmx-ext-sse.js`,
		`/static/fonts/SchibstedGrotesk-Variable.woff2`,
		`hx-ext="sse"`,
		`sse-connect="/api/v1/events"`,
		`data-timer`,                            // live-timer script hook present
		`id="content"`,                          // body slot rendered
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("Base missing %q", w)
		}
	}
	// Hard guarantee: NO external origins.
	for _, bad := range []string{"unpkg.com", "googleapis.com", "gstatic.com", "fontshare.com", "cdn.tailwindcss.com"} {
		if strings.Contains(out, bad) {
			t.Errorf("Base must be offline but references %q", bad)
		}
	}
}

func TestBase_PreloadsLesesaalFonts(t *testing.T) {
	out := render(t, components.Base("t", templ.NopComponent))
	if !strings.Contains(out, "/static/fonts/SchibstedGrotesk-Variable.woff2") {
		t.Fatalf("Schibsted preload missing:\n%s", out)
	}
	for _, gone := range []string{"ClashDisplay", "Inter-Variable"} {
		if strings.Contains(out, gone) {
			t.Fatalf("stale font reference %q still present", gone)
		}
	}
}

func TestBase_LightIsHome_NoFacetsNoToggle(t *testing.T) {
	out := render(t, components.Base("t", templ.NopComponent))
	if !strings.Contains(out, `data-theme','light'`) && !strings.Contains(out, `data-theme", "light"`) {
		t.Fatalf("no-flash script does not force light:\n%s", out)
	}
	for _, gone := range []string{"kristall-facets", "toggleTheme", "flow-theme"} {
		if strings.Contains(out, gone) {
			t.Fatalf("kristall remnant %q still present", gone)
		}
	}
}
