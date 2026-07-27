package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// TestBase_AssetURLsCarryVersionQuery — Lesesaal L4 Task 7: once the asset
// fingerprint is known (SetAssetVersion, wired once in
// httpserver.Server.Routes() with webui.AssetVersion()), the base hull must
// render every /static/ URL with "?v=<hash>" — a bare "/static/app.css"
// (no query) is exactly the PROD bug this task fixes (Cloudflare's 4h edge
// cache serving stale CSS/JS against fresh HTML after a deploy).
func TestBase_AssetURLsCarryVersionQuery(t *testing.T) {
	components.SetAssetVersion("fingerprint1")
	t.Cleanup(func() { components.SetAssetVersion("") })

	out := render(t, components.Base("Test", templ.NopComponent))

	for _, want := range []string{
		`/static/app.css?v=fingerprint1`,
		`/static/chroma.css?v=fingerprint1`,
		`/static/fonts/SchibstedGrotesk-Variable.woff2?v=fingerprint1`,
		`/static/fonts/JetBrainsMono-Variable.woff2?v=fingerprint1`,
		`/static/vendor/htmx.min.js?v=fingerprint1`,
		`/static/vendor/htmx-ext-sse.js?v=fingerprint1`,
		`/static/scrollspy.js?v=fingerprint1`,
		`/static/toc.js?v=fingerprint1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Base missing %q\n%s", want, out)
		}
	}
	// The bug this task fixes: a bare, un-versioned static URL.
	if strings.Contains(out, `href="/static/app.css"`) {
		t.Error("Base still renders un-versioned /static/app.css")
	}
}

// TestBase_AssetURLs_NoVersionSet_StillServable — without SetAssetVersion
// (unit tests that never boot httpserver.Server.Routes()) the hull must
// still render valid, servable URLs — the fallback has no cache-busting,
// but nothing 404s.
func TestBase_AssetURLs_NoVersionSet_StillServable(t *testing.T) {
	components.SetAssetVersion("")
	out := render(t, components.Base("Test", templ.NopComponent))
	if !strings.Contains(out, `/static/app.css`) {
		t.Fatalf("Base missing /static/app.css entirely:\n%s", out)
	}
}
