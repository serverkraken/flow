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

func TestBaseHullIsOfflineAndThemed(t *testing.T) {
	body := templ.Raw(`<p id="content">hallo</p>`)
	out := render(t, components.Base("Test", body))

	wants := []string{
		`<!doctype html>`,                       // templ lowercases the doctype
		`lang="de"`,
		`/static/app.css`,
		`/static/vendor/htmx.min.js`,            // local, NOT unpkg
		`/static/vendor/htmx-ext-sse.js`,
		`/static/fonts/Inter-Variable.woff2`,
		`flow-theme`,                            // no-flash boot script reads localStorage key
		`window.toggleTheme`,                    // theme-sync script
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

func TestThemeTogglePressableAndLabeled(t *testing.T) {
	out := render(t, components.ThemeToggle())
	for _, w := range []string{`data-theme-toggle`, `aria-pressed="false"`, `onclick="toggleTheme()"`, `toggle-sun`, `toggle-moon`, `<svg`, `viewBox="0 0 24 24"`} {
		if !strings.Contains(out, w) {
			t.Errorf("ThemeToggle missing %q", w)
		}
	}
}

func TestBase_KristallFacets(t *testing.T) {
	// mockup-normative facet layer: token-tinted polygons + soft radial pools
	body := templ.Raw(`<p id="content">hallo</p>`)
	out := render(t, components.Base("Test", body))
	for _, want := range []string{`class="kristall-facets"`, `fill-opacity=".022"`, `stroke-opacity=".04"`, `url(#kfacet-glow)`} {
		if !strings.Contains(out, want) {
			t.Errorf("facets layer missing %q", want)
		}
	}
}
